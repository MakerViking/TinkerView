package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/MakerViking/TinkerView/internal/camera"
	"github.com/MakerViking/TinkerView/internal/devices"
)

// deviceBackend gives the REST layer the richer device operations that the
// hub.CommandHandler interface (shared with the WebSocket) doesn't carry: reading
// a stored printer/camera for the edit forms, and the credential-clearing camera
// edit.
type deviceBackend struct {
	printers *devices.Manager
	cameras  *camera.Manager
}

// SetDevices enables GET /api/printers/{id}, GET /api/cameras/{name} and the
// full-featured camera edit. Without it those endpoints return 503 and
// PUT /api/cameras/{name} falls back to the WS-equivalent EditCamera.
func (a *API) SetDevices(p *devices.Manager, c *camera.Manager) {
	a.devs = &deviceBackend{printers: p, cameras: c}
}

// writeErr emits the standard error body {"error": "<message>"}.
func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// writeJSONStatus is writeJSON with an explicit status code.
func writeJSONStatus(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// printerGet returns a stored printer for the edit form. Secrets (API key, Bambu
// access code) are never returned, only hasApiKey / hasAccessCode.
func (a *API) printerGet(w http.ResponseWriter, r *http.Request) {
	if a.devs == nil {
		writeErr(w, http.StatusServiceUnavailable, "device editing is not available")
		return
	}
	rec, ok := a.devs.printers.PrinterRecord(r.PathValue("id"))
	if !ok {
		writeErr(w, http.StatusNotFound, "unknown or read-only printer")
		return
	}
	writeJSON(w, rec.View())
}

func (a *API) printerAdd(w http.ResponseWriter, r *http.Request) {
	var rec devices.PrinterRecord // decodes apiKey/accessCode; never marshalled back with them
	if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if err := a.cmd.AddPrinter(rec); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, rec.View())
}

func (a *API) printerEdit(w http.ResponseWriter, r *http.Request) {
	var rec devices.PrinterRecord
	if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	id := r.PathValue("id")
	if err := a.cmd.EditPrinter(id, rec); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if a.devs != nil {
		if cur, ok := a.devs.printers.PrinterRecord(id); ok {
			writeJSON(w, cur.View())
			return
		}
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (a *API) printerDelete(w http.ResponseWriter, r *http.Request) {
	if err := a.cmd.RemovePrinter(r.PathValue("id")); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// cameraGet describes a camera for the edit form: source kind/URL (credentials
// stripped), username, whether a password is set, and what may be edited.
func (a *API) cameraGet(w http.ResponseWriter, r *http.Request) {
	if a.devs == nil {
		writeErr(w, http.StatusServiceUnavailable, "device editing is not available")
		return
	}
	info, ok := a.devs.cameras.Info(r.Context(), r.PathValue("name"))
	if !ok {
		writeErr(w, http.StatusNotFound, "unknown camera, or it has no editable go2rtc stream")
		return
	}
	writeJSON(w, info)
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
		writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	if err := a.cmd.AddCamera(body.Name, body.SrcKind, body.SrcInput, body.Username, body.Password); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if a.devs != nil {
		if info, ok := a.devs.cameras.Info(r.Context(), body.Name); ok {
			writeJSON(w, info)
			return
		}
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// cameraEdit applies a camera edit. Omitted/empty username or password keep the
// stored value; clearCredentials removes both. A promoted camera's source change
// runs in the background (202); everything else is applied before returning (200).
func (a *API) cameraEdit(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NewName          string `json:"newName"`
		NewSrcKind       string `json:"newSrcKind"`
		NewSrcInput      string `json:"newSrcInput"`
		Username         string `json:"username"`
		Password         string `json:"password"`
		ClearCredentials bool   `json:"clearCredentials"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}
	name := r.PathValue("name")
	if a.devs == nil {
		if body.ClearCredentials {
			writeErr(w, http.StatusServiceUnavailable, "clearing credentials is not available")
			return
		}
		if err := a.cmd.EditCamera(name, body.NewName, body.NewSrcKind, body.NewSrcInput, body.Username, body.Password); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
		return
	}
	async, err := a.devs.cameras.Edit(name, camera.EditRequest{
		NewName: body.NewName, NewSrcKind: body.NewSrcKind, NewSrcInput: body.NewSrcInput,
		Username: body.Username, Password: body.Password, ClearCredentials: body.ClearCredentials,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	cur := name
	if nn := strings.TrimSpace(body.NewName); !async && nn != "" {
		cur = nn
	}
	code := http.StatusOK
	if async {
		code = http.StatusAccepted
	}
	if info, ok := a.devs.cameras.Info(r.Context(), cur); ok {
		writeJSONStatus(w, code, info)
		return
	}
	writeJSONStatus(w, code, map[string]bool{"ok": true})
}

func (a *API) cameraDelete(w http.ResponseWriter, r *http.Request) {
	if err := a.cmd.RemoveCamera(r.PathValue("name")); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
