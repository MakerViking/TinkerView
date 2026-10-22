package web

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/MakerViking/TinkerView/internal/adapter"
	"github.com/MakerViking/TinkerView/internal/devices"
	"github.com/MakerViking/TinkerView/internal/hub"
	"github.com/MakerViking/TinkerView/internal/notify"
	"github.com/MakerViking/TinkerView/internal/rules"
)

// FrigateConfigManager edits Frigate's global config from the UI — known license
// plates (lpr.known_plates) and the object-detection hardware (detectors/model/
// ffmpeg). Implemented by the camera manager; nil when Frigate config editing is off.
type FrigateConfigManager interface {
	KnownPlates(ctx context.Context) (map[string][]string, error)
	SetKnownPlates(plates map[string][]string) error
	DetectionSettings(ctx context.Context) (devices.DetectionSettings, bool)
	SetDetectionSettings(s devices.DetectionSettings) error
}

// API holds the REST mirror handlers. Most live state is delivered over the WS;
// these endpoints exist for initial paint, debugging, ntfy lock-screen action
// buttons (which can only do plain HTTP), and the Recognition tab's
// plate/notification-rule editors.
type API struct {
	version  string
	hub      *hub.Hub
	cmd      hub.CommandHandler
	notifier notify.Notifier
	frig     FrigateConfigManager // may be nil
	rules    *rules.Store         // may be nil
	devs     *deviceBackend       // set by SetDevices; nil -> the edit/inspect endpoints degrade
}

func NewAPI(version string, h *hub.Hub, cmd hub.CommandHandler, notifier notify.Notifier, frig FrigateConfigManager, ruleStore *rules.Store) *API {
	return &API{version: version, hub: h, cmd: cmd, notifier: notifier, frig: frig, rules: ruleStore}
}

// Register wires the REST routes onto mux (Go 1.22+ method+path patterns).
func (a *API) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/version", a.version_)
	mux.HandleFunc("GET /api/state", a.state)
	mux.HandleFunc("GET /api/health", a.health)
	mux.HandleFunc("POST /api/printers/{id}/pause", a.printerCmd(adapter.CmdPause))
	mux.HandleFunc("POST /api/printers/{id}/resume", a.printerCmd(adapter.CmdResume))
	mux.HandleFunc("POST /api/printers/{id}/cancel", a.printerCmd(adapter.CmdCancel))
	mux.HandleFunc("POST /api/printers/{id}/control", a.printerControl)
	mux.HandleFunc("POST /api/containers/{id}/restart", a.restart)
	mux.HandleFunc("POST /api/notify/test", a.notifyTest)
	// Device CRUD mirror of the WS commands (scripting/debugging).
	mux.HandleFunc("GET /api/printers/{id}", a.printerGet)
	mux.HandleFunc("POST /api/printers", a.printerAdd)
	mux.HandleFunc("PUT /api/printers/{id}", a.printerEdit)
	mux.HandleFunc("DELETE /api/printers/{id}", a.printerDelete)
	mux.HandleFunc("GET /api/cameras/{name}", a.cameraGet)
	mux.HandleFunc("POST /api/cameras", a.cameraAdd)
	mux.HandleFunc("PUT /api/cameras/{name}", a.cameraEdit)
	mux.HandleFunc("DELETE /api/cameras/{name}", a.cameraDelete)
	mux.HandleFunc("POST /api/cameras/{name}/promote", a.cameraPromote)
	mux.HandleFunc("POST /api/cameras/{name}/demote", a.cameraDemote)
	mux.HandleFunc("PUT /api/cameras/{name}/settings", a.cameraSettings)
	mux.HandleFunc("GET /api/cameras/{name}/settings", a.cameraSettingsGet)
	// Recognition tab: known license plates (Frigate lpr) + notification rules.
	mux.HandleFunc("GET /api/plates", a.platesGet)
	mux.HandleFunc("PUT /api/plates", a.platesPut)
	mux.HandleFunc("GET /api/notify-rules", a.rulesGet)
	mux.HandleFunc("PUT /api/notify-rules", a.rulesPut)
	// Detection tab: Frigate object-detection hardware (detectors/model/hwaccel).
	mux.HandleFunc("GET /api/detection", a.detectionGet)
	mux.HandleFunc("PUT /api/detection", a.detectionSet)
}

func (a *API) version_(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"version": a.version})
}

func (a *API) state(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, a.hub.Snapshot())
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	s := a.hub.Snapshot()
	writeJSON(w, map[string]any{
		"containers":    s.Containers,
		"disks":         s.Disks,
		"frigateOnline": s.FrigateOnline,
	})
}

func (a *API) printerCmd(cmd adapter.PrinterCommand) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.cmd == nil {
			http.Error(w, "no command handler", http.StatusServiceUnavailable)
			return
		}
		if err := a.cmd.PrinterCommand(r.Context(), r.PathValue("id"), cmd); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (a *API) printerControl(w http.ResponseWriter, r *http.Request) {
	var action adapter.PrinterAction
	if err := json.NewDecoder(r.Body).Decode(&action); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.cmd.PrinterControl(r.Context(), r.PathValue("id"), action); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) restart(w http.ResponseWriter, r *http.Request) {
	if a.cmd == nil {
		http.Error(w, "no command handler", http.StatusServiceUnavailable)
		return
	}
	if err := a.cmd.ContainerRestart(r.Context(), r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) cameraPromote(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Detect     bool `json:"detect"`
		Record     bool `json:"record"`
		RecordDays int  `json:"recordDays"`
		Snapshots  bool `json:"snapshots"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.cmd.PromoteCamera(r.PathValue("name"), body.Detect, body.Record, body.RecordDays, body.Snapshots); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusAccepted) // promote runs asynchronously
}

func (a *API) cameraSettingsGet(w http.ResponseWriter, r *http.Request) {
	s, ok := a.cmd.CameraSettings(r.PathValue("name"))
	if !ok {
		http.Error(w, "unknown camera", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s)
}

func (a *API) cameraSettings(w http.ResponseWriter, r *http.Request) {
	var s devices.CameraSettings
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.cmd.EditCameraSettings(r.PathValue("name"), s); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusAccepted) // applying to a promoted camera runs asynchronously
}

func (a *API) cameraDemote(w http.ResponseWriter, r *http.Request) {
	if err := a.cmd.DemoteCamera(r.PathValue("name")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (a *API) platesGet(w http.ResponseWriter, r *http.Request) {
	if a.frig == nil {
		http.Error(w, "Frigate config editing is not available", http.StatusServiceUnavailable)
		return
	}
	m, err := a.frig.KnownPlates(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if m == nil {
		m = map[string][]string{}
	}
	writeJSON(w, map[string]any{"plates": m})
}

func (a *API) platesPut(w http.ResponseWriter, r *http.Request) {
	if a.frig == nil {
		http.Error(w, "Frigate config editing is not available", http.StatusServiceUnavailable)
		return
	}
	var body struct {
		Plates map[string][]string `json:"plates"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.frig.SetKnownPlates(body.Plates); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusAccepted) // applying to Frigate (restart) runs asynchronously
}

func (a *API) detectionGet(w http.ResponseWriter, r *http.Request) {
	if a.frig == nil {
		http.Error(w, "Frigate config editing is not available", http.StatusServiceUnavailable)
		return
	}
	s, ok := a.frig.DetectionSettings(r.Context())
	if !ok {
		http.Error(w, "could not read the Frigate config", http.StatusBadGateway)
		return
	}
	writeJSON(w, s)
}

func (a *API) detectionSet(w http.ResponseWriter, r *http.Request) {
	if a.frig == nil {
		http.Error(w, "Frigate config editing is not available", http.StatusServiceUnavailable)
		return
	}
	var s devices.DetectionSettings
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.frig.SetDetectionSettings(s); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusAccepted) // applying to Frigate (restart) runs asynchronously
}

func (a *API) rulesGet(w http.ResponseWriter, r *http.Request) {
	if a.rules == nil {
		writeJSON(w, []rules.Rule{})
		return
	}
	writeJSON(w, a.rules.Get())
}

func (a *API) rulesPut(w http.ResponseWriter, r *http.Request) {
	if a.rules == nil {
		http.Error(w, "notification rules are not available", http.StatusServiceUnavailable)
		return
	}
	var rs []rules.Rule
	if err := json.NewDecoder(r.Body).Decode(&rs); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.rules.Set(rs); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) notifyTest(w http.ResponseWriter, r *http.Request) {
	if a.notifier == nil {
		http.Error(w, "no notifier configured", http.StatusServiceUnavailable)
		return
	}
	err := a.notifier.Send(r.Context(), notify.Notification{
		Title:    "Webcam Dashboard test",
		Message:  "Notifications are working ✅",
		Priority: 3,
		Tags:     []string{"bell"},
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("api: json encode failed", "err", err)
	}
}
