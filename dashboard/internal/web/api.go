package web

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/MakerViking/TinkerView/internal/adapter"
	"github.com/MakerViking/TinkerView/internal/devices"
	"github.com/MakerViking/TinkerView/internal/hub"
	"github.com/MakerViking/TinkerView/internal/notify"
)

// API holds the REST mirror handlers. Most live state is delivered over the WS;
// these endpoints exist for initial paint, debugging, and ntfy lock-screen action
// buttons (which can only do plain HTTP).
type API struct {
	version  string
	hub      *hub.Hub
	cmd      hub.CommandHandler
	notifier notify.Notifier
}

func NewAPI(version string, h *hub.Hub, cmd hub.CommandHandler, notifier notify.Notifier) *API {
	return &API{version: version, hub: h, cmd: cmd, notifier: notifier}
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
	mux.HandleFunc("POST /api/printers", a.printerAdd)
	mux.HandleFunc("PUT /api/printers/{id}", a.printerEdit)
	mux.HandleFunc("DELETE /api/printers/{id}", a.printerDelete)
	mux.HandleFunc("POST /api/cameras", a.cameraAdd)
	mux.HandleFunc("PUT /api/cameras/{name}", a.cameraEdit)
	mux.HandleFunc("DELETE /api/cameras/{name}", a.cameraDelete)
	mux.HandleFunc("POST /api/cameras/{name}/promote", a.cameraPromote)
	mux.HandleFunc("POST /api/cameras/{name}/demote", a.cameraDemote)
	mux.HandleFunc("PUT /api/cameras/{name}/settings", a.cameraSettings)
	mux.HandleFunc("GET /api/cameras/{name}/settings", a.cameraSettingsGet)
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

func (a *API) printerAdd(w http.ResponseWriter, r *http.Request) {
	var rec devices.PrinterRecord
	if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.cmd.AddPrinter(rec); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) printerEdit(w http.ResponseWriter, r *http.Request) {
	var rec devices.PrinterRecord
	if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.cmd.EditPrinter(r.PathValue("id"), rec); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) printerDelete(w http.ResponseWriter, r *http.Request) {
	if err := a.cmd.RemovePrinter(r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) cameraAdd(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string `json:"name"`
		SrcKind  string `json:"srcKind"`
		SrcInput string `json:"srcInput"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.cmd.AddCamera(body.Name, body.SrcKind, body.SrcInput, body.Username, body.Password); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) cameraEdit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NewName     string `json:"newName"`
		NewSrcKind  string `json:"newSrcKind"`
		NewSrcInput string `json:"newSrcInput"`
		Username    string `json:"username"`
		Password    string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := a.cmd.EditCamera(r.PathValue("name"), body.NewName, body.NewSrcKind, body.NewSrcInput, body.Username, body.Password); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) cameraDelete(w http.ResponseWriter, r *http.Request) {
	if err := a.cmd.RemoveCamera(r.PathValue("name")); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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
