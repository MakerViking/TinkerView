// Package prusalink implements adapter.Printer for Prusa printers via the native
// PrusaLink HTTP API. It speaks two dialects and auto-detects which the printer
// supports, locking onto the first that answers:
//
//   - v1 (firmware 4.7.0+): GET /api/v1/status (+ /api/v1/job on job change), HTTP
//     Digest auth (username "maker", password = the PrusaLink password/API key) —
//     the same API Home Assistant's prusalink integration uses.
//   - legacy (older firmware / OctoPrint-compat): GET /api/printer (+ /api/job),
//     gated by an X-Api-Key header with the same key. Reached as a fallback when
//     /api/v1 answers 401/403/404 (e.g. a Prusa XL on firmware whose /api/v1 path
//     isn't served), so the printer still shows up. Temps come from the
//     temperature.tool0/bed block (PrusaLink's telemetry.temp-nozzle reads a bogus
//     ~6 C while idle).
//
// Digest is implemented inline since Go's stdlib has no Digest support. Job control
// (pause/resume/cancel) is the ONLY control PrusaLink exposes on either dialect.
//
// Scope: built-in PrusaLink (MK4 / MK3.9 / MK3.5 / XL / MINI / Core One) and
// PrusaLink-on-a-Pi (MK2.5/MK3). A Prusa running real OctoPrint on a Pi should use
// the "octoprint" kind instead. PrusaLink's /api/v1 has no set-temp / move / gcode
// endpoints (those exist only on the fragile, firmware-dependent legacy /api/*
// OctoPrint-compat layer), so this adapter advertises no expanded Caps — only the
// pause/resume/cancel flags. See the handoff for the legacy-temp follow-up option.
package prusalink

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MakerViking/TinkerView/internal/adapter"
	"github.com/MakerViking/TinkerView/internal/bus"
)

// pollInterval matches the OctoPrint adapter — responsive progress, gentle on a Pi.
const pollInterval = 3 * time.Second

// digestUser is hardcoded in PrusaLink firmware on all modern models.
const digestUser = "maker"

// v1 temp_nozzle sanity bounds. Some PrusaLink firmware (e.g. Prusa XL stock
// 6.4.x) reports printer.temp_nozzle stuck at ~6 C over /api/v1 while the legacy
// temperature.tool0 block is correct. Below v1NozzleSaneMin we cross-read the
// legacy block; if it's hotter than the v1 value by more than v1NozzleDisagree the
// v1 reading is bogus and we switch this printer to legacy temps.
const (
	v1NozzleSaneMin  = 15.0
	v1NozzleDisagree = 8.0
)

// v1NozzleSuspect reports whether a v1 nozzle reading is low enough to warrant a
// legacy cross-check (a powered printer's hotend at idle indoors sits at ~room temp).
func v1NozzleSuspect(actual float64) bool { return actual < v1NozzleSaneMin }

// v1NozzleBogus reports whether the legacy reading contradicts the v1 reading enough
// to conclude the v1 nozzle temperature is stuck/wrong.
func v1NozzleBogus(v1, legacy float64) bool { return legacy-v1 > v1NozzleDisagree }

type Printer struct {
	bus      *bus.Bus
	id       string
	name     string
	camera   string
	httpBase string // http://printer-ip (PrusaLink serves /api/v1 at the root)
	uiURL    string
	pass     string // Digest password (carried in the record's APIKey field)
	managed  bool
	client   *http.Client

	// HTTP Digest challenge state (guarded by dmu).
	dmu        sync.Mutex
	dRealm     string
	dNonce     string
	dQop       string
	dOpaque    string
	dAlgorithm string
	dNC        uint64
	dHave      bool

	mu         sync.Mutex
	apiMode    string // "" (undetected) | "v1" (/api/v1 Digest) | "legacy" (/api/printer X-Api-Key)
	v1TempsBad bool   // sticky: this printer's /api/v1 temp_nozzle is bogus; read temps from legacy
	online     bool
	offlineMsg string
	pstate     adapter.PState
	nozzle     adapter.Heater
	bed        adapter.Heater
	progressPct float64
	etaSec     int
	elapsedSec int
	jobID      int
	lastJobID  int // job id whose filename is cached (avoid re-fetching /job each poll)
	filename   string
}

func New(b *bus.Bus, id, name, baseURL, apiKey, camera, uiURL string, managed bool, _ []string) *Printer {
	base := strings.TrimRight(baseURL, "/")
	if uiURL == "" {
		uiURL = base
	}
	return &Printer{
		bus: b, id: id, name: name, camera: camera,
		httpBase: base, uiURL: uiURL, pass: apiKey, managed: managed,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *Printer) ID() string   { return p.id }
func (p *Printer) Kind() string { return "prusalink" }

func (p *Printer) Snapshot() adapter.PrinterState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.toState()
}

// ----- lifecycle -----

func (p *Printer) Run(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("prusalink adapter panic recovered", "printer", p.id, "recover", r)
		}
	}()
	p.publish(p.Snapshot())
	p.poll(ctx)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

// poll dispatches to the detected dialect. While undetected it tries v1; on a
// 401/403/404 (v1 not served / Digest refused) it falls back to the legacy API and
// locks onto whichever answers. A transport failure (unreachable) does NOT switch
// dialects — it's the network, not the protocol.
func (p *Printer) poll(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("prusalink poll panic recovered", "printer", p.id, "recover", r)
		}
	}()
	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	if p.getMode() == "legacy" {
		if ok, code, err := p.pollLegacy(cctx); !ok {
			p.setOffline(offlineReason(code, err))
		}
		return
	}
	ok, code, err := p.pollV1(cctx)
	if ok {
		p.setMode("v1")
		return
	}
	if shouldFallback(code) {
		if ok2, code2, err2 := p.pollLegacy(cctx); ok2 {
			if p.getMode() != "legacy" {
				slog.Info("prusalink: /api/v1 unavailable, using legacy /api/printer (X-Api-Key)", "printer", p.id, "v1_status", code)
			}
			p.setMode("legacy")
			return
		} else {
			p.setOffline(offlineReason(code2, err2))
			return
		}
	}
	p.setOffline(offlineReason(code, err))
}

// pollV1 polls the /api/v1 dialect (Digest). Returns ok=false with the HTTP status
// so the caller can decide whether to fall back.
func (p *Printer) pollV1(ctx context.Context) (bool, int, error) {
	var st plStatus
	code, err := p.do(ctx, http.MethodGet, "/api/v1/status", &st)
	if err != nil {
		return false, code, err
	}
	pst := classifyPState(st.Printer.State)
	jobID, progress, eta, elapsed := 0, 0.0, 0, 0
	if st.Job != nil {
		jobID = st.Job.ID
		progress = float64(st.Job.Progress)
		if st.Job.TimeRemaining != nil {
			eta = *st.Job.TimeRemaining
		}
		elapsed = st.Job.TimePrinting
	}

	// status.job carries no filename — fetch /api/v1/job only when the job id changes,
	// keeping the previous name on a transient miss (don't strand the job nameless).
	p.mu.Lock()
	filename := p.filename
	lastJobID := p.lastJobID
	p.mu.Unlock()
	switch {
	case jobID == 0:
		filename = ""
		p.mu.Lock()
		p.lastJobID = 0
		p.mu.Unlock()
	case jobID != lastJobID:
		var jb plJob
		if c, e := p.do(ctx, http.MethodGet, "/api/v1/job", &jb); e == nil && c == http.StatusOK {
			filename = jb.File.DisplayName
			if filename == "" {
				filename = jb.File.Name
			}
			p.mu.Lock()
			p.lastJobID = jobID
			p.mu.Unlock()
		}
	}

	nozzle := adapter.Heater{Actual: float64(st.Printer.TempNozzle), Target: float64(st.Printer.TargetNozzle)}
	bed := adapter.Heater{Actual: float64(st.Printer.TempBed), Target: float64(st.Printer.TargetBed)}
	// Work around firmware that reports a bogus v1 temp_nozzle (Prusa XL stock):
	// cross-read the legacy temperature.tool0 block when the v1 nozzle looks too cold
	// (or once we've confirmed it's bad for this printer), and prefer it.
	p.mu.Lock()
	useLegacy := p.v1TempsBad
	p.mu.Unlock()
	if useLegacy || v1NozzleSuspect(float64(st.Printer.TempNozzle)) {
		var lp legacyPrinter
		if _, e := p.legacyGet(ctx, "/api/printer", &lp); e == nil {
			ltool := float64(lp.Temperature.Tool0.Actual)
			if !useLegacy && v1NozzleBogus(float64(st.Printer.TempNozzle), ltool) {
				p.mu.Lock()
				p.v1TempsBad = true
				p.mu.Unlock()
				useLegacy = true
				slog.Warn("prusalink: /api/v1 temp_nozzle is unreliable on this printer; using legacy temperature.tool0",
					"printer", p.id, "v1_nozzle", float64(st.Printer.TempNozzle), "legacy_tool0", ltool)
			}
			if useLegacy {
				nozzle = adapter.Heater{Actual: ltool, Target: float64(lp.Temperature.Tool0.Target)}
				bed = adapter.Heater{Actual: float64(lp.Temperature.Bed.Actual), Target: float64(lp.Temperature.Bed.Target)}
			}
		} else if useLegacy {
			// Known-bad v1 temps but legacy momentarily unreachable — keep the last good
			// readings rather than flash the bogus v1 value.
			p.mu.Lock()
			nozzle, bed = p.nozzle, p.bed
			p.mu.Unlock()
		}
	}
	p.commit(pst, nozzle, bed, progress, eta, elapsed, jobID, filename)
	return true, code, nil
}

// pollLegacy polls the legacy OctoPrint-compatible dialect (X-Api-Key). /api/job is
// best-effort (null when idle).
func (p *Printer) pollLegacy(ctx context.Context) (bool, int, error) {
	var pr legacyPrinter
	code, err := p.legacyGet(ctx, "/api/printer", &pr)
	if err != nil {
		return false, code, err
	}
	var jb legacyJob
	_, _ = p.legacyGet(ctx, "/api/job", &jb) // null/idle is fine

	pst := classifyLegacy(pr.State.Flags)
	// telemetry.temp-nozzle is unreliable (~6 C idle); the temperature.* block is correct.
	nozzle := adapter.Heater{Actual: float64(pr.Temperature.Tool0.Actual), Target: float64(pr.Temperature.Tool0.Target)}
	bed := adapter.Heater{Actual: float64(pr.Temperature.Bed.Actual), Target: float64(pr.Temperature.Bed.Target)}

	filename := ""
	if jb.Job != nil {
		filename = jb.Job.File.DisplayName
		if filename == "" {
			filename = jb.Job.File.Name
		}
	}
	progress, eta, elapsed := 0.0, 0, 0
	if jb.Progress != nil {
		// OctoPrint-compat completion is 0..100 already — no scaling (verified for the
		// octoprint adapter; a ×100 here would inflate a just-started 0.5% to 50%).
		progress = clampPct(float64(jb.Progress.Completion))
		elapsed = int(jb.Progress.PrintTime)
		if jb.Progress.PrintTimeLeft > 0 {
			eta = int(jb.Progress.PrintTimeLeft)
		}
	}
	// Legacy control posts a command (no numeric job id); use 1 as an "active" marker
	// so any id!=0 guard passes — Command routes to legacyCommand in legacy mode.
	jobID := 0
	if jb.Job != nil {
		jobID = 1
	}
	p.commit(pst, nozzle, bed, progress, eta, elapsed, jobID, filename)
	return true, http.StatusOK, nil
}

// commit stores the mapped state under the lock and publishes it.
func (p *Printer) commit(pst adapter.PState, nozzle, bed adapter.Heater, progress float64, eta, elapsed, jobID int, filename string) {
	p.mu.Lock()
	p.online = true
	p.offlineMsg = ""
	p.pstate = pst
	p.nozzle = nozzle
	p.bed = bed
	p.progressPct = progress
	p.etaSec = eta
	p.elapsedSec = elapsed
	p.jobID = jobID
	p.filename = filename
	out := p.toState()
	p.mu.Unlock()
	p.publish(out)
}

func (p *Printer) getMode() string  { p.mu.Lock(); defer p.mu.Unlock(); return p.apiMode }
func (p *Printer) setMode(m string) { p.mu.Lock(); p.apiMode = m; p.mu.Unlock() }

// shouldFallback reports whether a v1 failure means "try the legacy API" (the path
// isn't served or Digest was refused) rather than "the printer is unreachable".
func shouldFallback(code int) bool {
	return code == http.StatusUnauthorized || code == http.StatusForbidden || code == http.StatusNotFound
}

// offlineReason turns a failed /api/v1/status call into a user-facing message that
// distinguishes the real cause — so an operator who can't see the logs (e.g. a
// friend running their own instance) can tell auth from network from a schema/
// firmware mismatch right from the printer tile. do() returns the HTTP status
// alongside the error: code 0 = transport failure, 401 = auth, 200 = the request
// succeeded but the JSON didn't decode (an unexpected response shape).
func offlineReason(code int, err error) string {
	switch {
	case code == http.StatusUnauthorized:
		return "PrusaLink rejected the credentials (HTTP 401) — check the password (it's the PrusaLink API key; username is always \"maker\")"
	case code == http.StatusOK:
		return "PrusaLink returned an unexpected response format: " + err.Error()
	case code >= 400:
		return fmt.Sprintf("PrusaLink error: HTTP %d", code)
	default:
		return "PrusaLink unreachable: " + err.Error()
	}
}

func (p *Printer) setOffline(msg string) {
	p.mu.Lock()
	p.online = false
	p.offlineMsg = msg
	st := p.toState()
	p.mu.Unlock()
	p.publish(st)
}

func (p *Printer) publish(st adapter.PrinterState) {
	p.bus.Publish(bus.Event{Source: bus.SourcePrinter, Kind: bus.KindPrinterState, ID: p.id, Ts: time.Now(), Payload: st})
}

// toState maps the cached PrusaLink state to the normalized PrinterState. Caller holds p.mu.
func (p *Printer) toState() adapter.PrinterState {
	s := adapter.PrinterState{ID: p.id, Kind: "prusalink", Name: p.name, Camera: p.camera, UIURL: p.uiURL, Online: p.online, Managed: p.managed}
	if !p.online {
		s.State = adapter.Offline
		s.Message = p.offlineMsg
		return s
	}
	s.State = p.pstate
	s.Filename = p.filename
	s.ProgressPct = p.progressPct // PrusaLink reports 0..100 already — no scaling
	s.EtaSec = p.etaSec
	s.ElapsedSec = p.elapsedSec
	s.Nozzle = p.nozzle
	s.Bed = p.bed
	switch p.pstate {
	case adapter.Printing:
		s.Flags = adapter.Flags{CanPause: true, CanCancel: true}
	case adapter.Paused:
		s.Flags = adapter.Flags{CanResume: true, CanCancel: true}
	}
	// Caps intentionally nil: PrusaLink /api/v1 exposes no temp/move/gcode controls,
	// only job pause/resume/cancel (surfaced via Flags above).
	return s
}

// classifyPState maps PrusaLink's 9-value printer.state enum to the normalized PState.
// Reachability (not state) decides Offline — there is no OFFLINE wire value.
func classifyPState(state string) adapter.PState {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "PRINTING":
		return adapter.Printing
	case "PAUSED":
		return adapter.Paused
	case "FINISHED":
		return adapter.Complete
	case "STOPPED":
		return adapter.Cancelled
	case "ERROR", "ATTENTION": // ATTENTION (runout/MMU) flagged as error for UI visibility
		return adapter.Errored
	default: // IDLE | READY | BUSY | unknown
		return adapter.Idle
	}
}

// ----- control (job only) -----

func (p *Printer) Command(ctx context.Context, cmd adapter.PrinterCommand) error {
	if p.getMode() == "legacy" {
		return p.legacyCommand(ctx, cmd)
	}
	id := p.currentJobID()
	if id == 0 {
		return fmt.Errorf("no active job to %s", cmd)
	}
	switch cmd {
	case adapter.CmdPause:
		_, err := p.do(ctx, http.MethodPut, fmt.Sprintf("/api/v1/job/%d/pause", id), nil)
		return err
	case adapter.CmdResume:
		_, err := p.do(ctx, http.MethodPut, fmt.Sprintf("/api/v1/job/%d/resume", id), nil)
		return err
	case adapter.CmdCancel:
		// DELETE stops/cancels the job (there is no /stop path; not resumable).
		_, err := p.do(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/job/%d", id), nil)
		return err
	default:
		return fmt.Errorf("unsupported command %q", cmd)
	}
}

func (p *Printer) currentJobID() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.jobID
}

func (p *Printer) SetTemp(context.Context, string, float64) error {
	return fmt.Errorf("PrusaLink does not support setting temperatures over its /api/v1 API")
}

func (p *Printer) Control(_ context.Context, a adapter.PrinterAction) error {
	return fmt.Errorf("action %q is not supported by PrusaLink", a.Op)
}

// ----- response types -----

type plStatus struct {
	Printer plStatusPrinter `json:"printer"`
	Job     *plStatusJob    `json:"job"` // pointer: absent/null when no active job
}

type plStatusPrinter struct {
	State        string    `json:"state"` // IDLE|BUSY|PRINTING|PAUSED|FINISHED|STOPPED|ERROR|ATTENTION|READY
	TempNozzle   flexFloat `json:"temp_nozzle"`
	TargetNozzle flexFloat `json:"target_nozzle"`
	TempBed      flexFloat `json:"temp_bed"`
	TargetBed    flexFloat `json:"target_bed"`
}

type plStatusJob struct {
	ID            int       `json:"id"`
	Progress      flexFloat `json:"progress"`       // 0..100
	TimeRemaining *int      `json:"time_remaining"` // seconds, nullable
	TimePrinting  int       `json:"time_printing"`  // seconds elapsed
}

// flexFloat accepts either a JSON number or a quoted-number string. PrusaLink's
// /api/v1 normally returns numbers, but tolerating a stringified telemetry value
// means one oddly-typed field on an unfamiliar firmware can't fail the whole
// status decode (which would otherwise drop the printer to "offline"). A blank or
// null value decodes to 0.
type flexFloat float64

func (f *flexFloat) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "" || s == "null" {
		return nil
	}
	if s[0] == '"' {
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		str = strings.TrimSpace(str)
		if str == "" {
			return nil
		}
		v, err := strconv.ParseFloat(str, 64)
		if err != nil {
			return err
		}
		*f = flexFloat(v)
		return nil
	}
	var v float64
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*f = flexFloat(v)
	return nil
}

type plJob struct {
	ID   int `json:"id"`
	File struct {
		Name        string `json:"name"`         // short 8.3
		DisplayName string `json:"display_name"` // human-readable (preferred)
	} `json:"file"`
}

// ----- legacy (OctoPrint-compatible) dialect: /api/printer + /api/job, X-Api-Key -----

type legacyPrinter struct {
	Temperature struct {
		Tool0 legacyHeater `json:"tool0"`
		Bed   legacyHeater `json:"bed"`
	} `json:"temperature"`
	State struct {
		Text  string      `json:"text"`
		Flags legacyFlags `json:"flags"`
	} `json:"state"`
}

type legacyFlags struct {
	Paused   bool `json:"paused"`
	Printing bool `json:"printing"`
	Error    bool `json:"error"`
}

type legacyHeater struct {
	Actual flexFloat `json:"actual"`
	Target flexFloat `json:"target"`
}

type legacyJob struct {
	Job *struct {
		File struct {
			Name        string `json:"name"`
			DisplayName string `json:"display_name"`
		} `json:"file"`
	} `json:"job"`
	Progress *struct {
		Completion    flexFloat `json:"completion"` // 0..100
		PrintTime     float64   `json:"printTime"`
		PrintTimeLeft float64   `json:"printTimeLeft"`
	} `json:"progress"`
}

// classifyLegacy maps the OctoPrint-compat state flags to the normalized PState.
// The legacy API has no distinct complete/cancelled flag (it returns to operational),
// so a finished/stopped print reads as Idle.
func classifyLegacy(f legacyFlags) adapter.PState {
	switch {
	case f.Printing:
		return adapter.Printing
	case f.Paused:
		return adapter.Paused
	case f.Error:
		return adapter.Errored
	default:
		return adapter.Idle
	}
}

func clampPct(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// legacyGet issues an X-Api-Key GET and decodes the body, returning the HTTP status
// (so the dispatcher can tell auth/not-found from a transport error).
func (p *Printer) legacyGet(ctx context.Context, path string, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.httpBase+path, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("X-Api-Key", p.pass)
	resp, err := p.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return resp.StatusCode, fmt.Errorf("GET %s: HTTP %d", path, resp.StatusCode)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

// legacyCommand issues pause/resume/cancel via the OctoPrint-compatible POST /api/job.
func (p *Printer) legacyCommand(ctx context.Context, cmd adapter.PrinterCommand) error {
	var body any
	switch cmd {
	case adapter.CmdPause:
		body = map[string]string{"command": "pause", "action": "pause"}
	case adapter.CmdResume:
		body = map[string]string{"command": "pause", "action": "resume"}
	case adapter.CmdCancel:
		body = map[string]string{"command": "cancel"}
	default:
		return fmt.Errorf("unsupported command %q", cmd)
	}
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.httpBase+"/api/job", bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", p.pass)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("POST /api/job: HTTP %d", resp.StatusCode)
	}
	return nil
}

// ----- HTTP with inline Digest auth -----

func (p *Printer) do(ctx context.Context, method, path string, out any) (int, error) {
	resp, err := p.attempt(ctx, method, path)
	if err != nil {
		return 0, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		www := resp.Header.Get("WWW-Authenticate")
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if www != "" {
			p.setChallenge(www) // parse realm/nonce/qop, then retry once with the response
			resp, err = p.attempt(ctx, method, path)
			if err != nil {
				return 0, err
			}
		}
	}
	defer resp.Body.Close()
	code := resp.StatusCode
	if code == http.StatusNoContent {
		return code, nil
	}
	if code >= 300 {
		return code, fmt.Errorf("prusalink %s %s: HTTP %d", method, path, code)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return code, err
		}
	}
	return code, nil
}

func (p *Printer) attempt(ctx context.Context, method, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, p.httpBase+path, nil)
	if err != nil {
		return nil, err
	}
	if h := p.authHeader(method, path); h != "" {
		req.Header.Set("Authorization", h)
	}
	return p.client.Do(req)
}

// authHeader builds the Digest Authorization header for one request, or "" if no
// challenge has been seen yet (the first request goes out unauthenticated, gets a
// 401 + challenge, and do() retries).
func (p *Printer) authHeader(method, uri string) string {
	p.dmu.Lock()
	defer p.dmu.Unlock()
	if !p.dHave {
		return ""
	}
	p.dNC++
	nc := fmt.Sprintf("%08x", p.dNC)
	cnonce := randHex(8)
	ha1 := md5hex(digestUser + ":" + p.dRealm + ":" + p.pass)
	// MD5-sess folds the nonce + client nonce into HA1. Plain MD5 (what current
	// PrusaLink uses) leaves HA1 as-is; handling -sess too guards against a firmware
	// build that negotiates it (otherwise every request would 401 → "offline").
	if strings.EqualFold(p.dAlgorithm, "MD5-sess") {
		ha1 = md5hex(ha1 + ":" + p.dNonce + ":" + cnonce)
	}
	ha2 := md5hex(method + ":" + uri)

	qop := ""
	for _, q := range strings.Split(p.dQop, ",") {
		if strings.TrimSpace(q) == "auth" {
			qop = "auth"
			break
		}
	}
	var response string
	if qop == "auth" {
		response = md5hex(strings.Join([]string{ha1, p.dNonce, nc, cnonce, qop, ha2}, ":"))
	} else {
		response = md5hex(ha1 + ":" + p.dNonce + ":" + ha2)
	}

	var b strings.Builder
	fmt.Fprintf(&b, `Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s"`,
		digestUser, p.dRealm, p.dNonce, uri, response)
	if p.dAlgorithm != "" {
		fmt.Fprintf(&b, ", algorithm=%s", p.dAlgorithm)
	}
	if qop == "auth" {
		fmt.Fprintf(&b, `, qop=auth, nc=%s, cnonce="%s"`, nc, cnonce)
	}
	if p.dOpaque != "" {
		fmt.Fprintf(&b, `, opaque="%s"`, p.dOpaque)
	}
	return b.String()
}

func (p *Printer) setChallenge(www string) {
	m := parseChallenge(www)
	if m == nil {
		return
	}
	p.dmu.Lock()
	p.dRealm = m["realm"]
	p.dNonce = m["nonce"]
	p.dQop = m["qop"]
	p.dOpaque = m["opaque"]
	p.dAlgorithm = m["algorithm"]
	p.dNC = 0
	p.dHave = p.dNonce != ""
	p.dmu.Unlock()
}

// parseChallenge parses the key=value pairs of a `Digest ...` WWW-Authenticate
// header, respecting quoted values (which may contain commas).
func parseChallenge(h string) map[string]string {
	i := strings.Index(h, "Digest ")
	if i < 0 {
		return nil
	}
	h = h[i+len("Digest "):]
	m := map[string]string{}
	inQuote := false
	start := 0
	flush := func(seg string) {
		kv := strings.SplitN(strings.TrimSpace(seg), "=", 2)
		if len(kv) == 2 {
			m[strings.TrimSpace(kv[0])] = strings.Trim(strings.TrimSpace(kv[1]), `"`)
		}
	}
	for j := 0; j < len(h); j++ {
		switch h[j] {
		case '"':
			inQuote = !inQuote
		case ',':
			if !inQuote {
				flush(h[start:j])
				start = j + 1
			}
		}
	}
	flush(h[start:])
	return m
}

func md5hex(s string) string {
	sum := md5.Sum([]byte(s)) // MD5 is mandated by the Digest auth scheme, not a security choice
	return hex.EncodeToString(sum[:])
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return strings.Repeat("0", n*2)
	}
	return hex.EncodeToString(b)
}
