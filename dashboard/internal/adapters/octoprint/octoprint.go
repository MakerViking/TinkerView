// Package octoprint implements adapter.Printer for OctoPrint via its REST API.
// OctoPrint has no push protocol we rely on here, so live state is polled on a
// short interval (/api/job for progress + state, /api/printer for temperatures +
// state flags); control uses the documented POST endpoints (/api/job,
// /api/printer/tool|bed|printhead|command) with the X-Api-Key header. All control
// safety gating (idle-gate, min-extrude-temp, macro allowlist, clamps) mirrors the
// Moonraker adapter — the adapter is the only layer that knows live state.
package octoprint

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/MakerViking/TinkerView/internal/adapter"
	"github.com/MakerViking/TinkerView/internal/adapters/printerutil"
	"github.com/MakerViking/TinkerView/internal/bus"
)

// pollInterval is how often we re-read OctoPrint while running. Short enough for a
// responsive progress bar, gentle enough for a LAN OctoPrint on a Pi.
const pollInterval = 3 * time.Second

type Printer struct {
	bus      *bus.Bus
	id       string
	name     string
	camera   string
	httpBase string // http://ip (OctoPrint root)
	uiURL    string // web UI to open on click; defaults to httpBase
	apiKey   string
	managed  bool
	macros   []string // curated macro allowlist (gcode sent verbatim); empty = none
	client   *http.Client

	mu             sync.Mutex
	online         bool
	offlineMsg     string
	pstate         adapter.PState
	printing       bool
	paused         bool
	nozzle         adapter.Heater
	bed            adapter.Heater
	filename       string
	progressPct    float64
	etaSec         int
	elapsedSec     int
	minExtrudeTemp float64
}

func New(b *bus.Bus, id, name, baseURL, apiKey, camera, uiURL string, managed bool, macros []string) *Printer {
	base := strings.TrimRight(baseURL, "/")
	if uiURL == "" {
		uiURL = base
	}
	return &Printer{
		bus: b, id: id, name: name, camera: camera,
		httpBase: base, uiURL: uiURL, apiKey: apiKey, managed: managed, macros: macros,
		client:         &http.Client{Timeout: 10 * time.Second},
		minExtrudeTemp: printerutil.DefaultMinExtrudeTemp,
	}
}

func (p *Printer) ID() string   { return p.id }
func (p *Printer) Kind() string { return "octoprint" }

func (p *Printer) Snapshot() adapter.PrinterState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.toState()
}

// ----- lifecycle -----

func (p *Printer) Run(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("octoprint adapter panic recovered", "printer", p.id, "recover", r)
		}
	}()
	p.publish(p.Snapshot()) // show the printer immediately (offline until first poll)
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

// poll reads /api/job (always available when the server is reachable) and, when the
// printer is operational, /api/printer for temperatures + precise state flags. It
// updates the cached state and publishes once. Wrapped in recover so a transient
// decode panic can't kill the poll loop.
func (p *Printer) poll(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("octoprint poll panic recovered", "printer", p.id, "recover", r)
		}
	}()
	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	var job octoJob
	if _, err := p.getJSON(cctx, "/api/job", &job); err != nil {
		p.setOffline("OctoPrint unreachable")
		return
	}

	var pr octoPrinter
	haveFlags := false
	if status, err := p.getJSON(cctx, "/api/printer", &pr); err == nil {
		haveFlags = true
	} else if status != http.StatusConflict {
		// 409 just means the printer port is closed (not operational) — expected,
		// fall back to job.State. Any other error: leave temps stale, still use job.
		slog.Debug("octoprint /api/printer failed", "printer", p.id, "status", status, "err", err)
	}

	pst, connected := classifyState(job.State)
	if haveFlags {
		pst, connected = refineFromFlags(pr.State.Flags, pst, connected)
	}

	p.mu.Lock()
	p.online = connected
	if connected {
		p.offlineMsg = ""
	} else if strings.TrimSpace(job.State) != "" {
		p.offlineMsg = "Printer " + strings.ToLower(job.State)
	} else {
		p.offlineMsg = "Printer not connected to OctoPrint"
	}
	p.pstate = pst
	p.printing = pst == adapter.Printing
	p.paused = pst == adapter.Paused
	if haveFlags {
		if pr.Temperature.Tool0 != nil {
			p.nozzle = adapter.Heater{Actual: pr.Temperature.Tool0.Actual, Target: pr.Temperature.Tool0.Target}
		}
		if pr.Temperature.Bed != nil {
			p.bed = adapter.Heater{Actual: pr.Temperature.Bed.Actual, Target: pr.Temperature.Bed.Target}
		}
	}
	p.filename = job.Job.File.Name
	// OctoPrint already reports completion on a 0..100 percent scale (its core
	// multiplies the raw 0..1 fraction by 100 before serialization), so assign it
	// directly — do NOT multiply again. (The docs' example value is stale/misleading.)
	if job.Progress.Completion != nil {
		p.progressPct = *job.Progress.Completion
	} else {
		p.progressPct = 0
	}
	if job.Progress.PrintTimeLeft != nil {
		p.etaSec = *job.Progress.PrintTimeLeft
	} else {
		p.etaSec = 0
	}
	if job.Progress.PrintTime != nil {
		p.elapsedSec = *job.Progress.PrintTime
	} else {
		p.elapsedSec = 0
	}
	st := p.toState()
	p.mu.Unlock()
	p.publish(st)
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

// toState maps the cached OctoPrint state to the normalized PrinterState. Caller holds p.mu.
func (p *Printer) toState() adapter.PrinterState {
	s := adapter.PrinterState{ID: p.id, Kind: "octoprint", Name: p.name, Camera: p.camera, UIURL: p.uiURL, Online: p.online, Managed: p.managed}
	if !p.online {
		s.State = adapter.Offline
		s.Message = p.offlineMsg
		return s
	}
	s.State = p.pstate
	s.Filename = p.filename
	s.ProgressPct = p.progressPct
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
	caps := adapter.Caps{SetTemps: true, Move: true, EmergencyStop: true}
	if len(p.macros) > 0 {
		caps.Macros = p.macros
	}
	s.Caps = &caps
	return s
}

// ----- control -----

func (p *Printer) Command(ctx context.Context, cmd adapter.PrinterCommand) error {
	switch cmd {
	case adapter.CmdPause:
		return p.postJSON(ctx, "/api/job", map[string]any{"command": "pause", "action": "pause"})
	case adapter.CmdResume:
		return p.postJSON(ctx, "/api/job", map[string]any{"command": "pause", "action": "resume"})
	case adapter.CmdCancel:
		return p.postJSON(ctx, "/api/job", map[string]any{"command": "cancel"})
	default:
		return fmt.Errorf("unsupported command %q", cmd)
	}
}

func (p *Printer) SetTemp(ctx context.Context, heater string, target float64) error {
	switch heater {
	case "bed":
		return p.postJSON(ctx, "/api/printer/bed", map[string]any{"command": "target", "target": target})
	case "nozzle", "extruder", "":
		return p.postJSON(ctx, "/api/printer/tool", map[string]any{"command": "target", "targets": map[string]any{"tool0": target}})
	case "chamber":
		return fmt.Errorf("this printer has no chamber heater")
	default:
		return fmt.Errorf("unsupported heater %q", heater)
	}
}

// Control dispatches an expanded action with the same safety gating as Moonraker.
func (p *Printer) Control(ctx context.Context, a adapter.PrinterAction) error {
	switch a.Op {
	case adapter.ActEmergencyStop:
		return p.postJSON(ctx, "/api/printer/command", map[string]any{"commands": []string{"M112"}})
	case adapter.ActHome:
		if !p.isIdle() {
			return printerutil.ErrNotIdle
		}
		return p.postJSON(ctx, "/api/printer/printhead", map[string]any{"command": "home", "axes": homeAxes(a.Axes)})
	case adapter.ActMove:
		if !p.isIdle() {
			return printerutil.ErrNotIdle
		}
		ax := strings.ToLower(a.Axis)
		if ax != "x" && ax != "y" && ax != "z" {
			return fmt.Errorf("invalid axis %q", a.Axis)
		}
		body := map[string]any{"command": "jog", "absolute": false, "speed": int(printerutil.ClampFeed(a.Feed, ax))}
		body[ax] = printerutil.ClampAbs(a.Dist, 100)
		return p.postJSON(ctx, "/api/printer/printhead", body)
	case adapter.ActExtrude:
		if !p.isIdle() {
			return printerutil.ErrNotIdle
		}
		if p.nozzleTemp() < p.minExtrudeTemp {
			return fmt.Errorf("nozzle below %.0f°C — heat it before extruding", p.minExtrudeTemp)
		}
		return p.postJSON(ctx, "/api/printer/tool", map[string]any{"command": "extrude", "amount": printerutil.ClampAbs(a.Dist, 100), "speed": int(printerutil.ClampFeed(a.Feed, "e"))})
	case adapter.ActMacro:
		if !p.macroAllowed(a.Name) {
			return fmt.Errorf("macro %q is not in the allowed list", a.Name)
		}
		return p.postJSON(ctx, "/api/printer/command", map[string]any{"commands": []string{a.Name}})
	default:
		return fmt.Errorf("unsupported action %q", a.Op)
	}
}

func (p *Printer) isIdle() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.printing && !p.paused
}

func (p *Printer) nozzleTemp() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.nozzle.Actual
}

// macroAllowed validates a macro name is in the curated allowlist (exact,
// case-insensitive) and contains no injection characters or extra tokens.
func (p *Printer) macroAllowed(name string) bool {
	return printerutil.MacroAllowed(name, p.macros)
}

// ----- REST plumbing -----

type octoHeater struct {
	Actual float64 `json:"actual"`
	Target float64 `json:"target"`
}

type octoFlags struct {
	Operational   bool `json:"operational"`
	Printing      bool `json:"printing"`
	Paused        bool `json:"paused"`
	Pausing       bool `json:"pausing"`
	Cancelling    bool `json:"cancelling"`
	Error         bool `json:"error"`
	ClosedOrError bool `json:"closedOrError"`
	Ready         bool `json:"ready"`
	Finishing     bool `json:"finishing"`
}

type octoPrinter struct {
	Temperature struct {
		Tool0 *octoHeater `json:"tool0"`
		Bed   *octoHeater `json:"bed"`
	} `json:"temperature"`
	State struct {
		Text  string    `json:"text"`
		Flags octoFlags `json:"flags"`
	} `json:"state"`
}

type octoJob struct {
	State string `json:"state"`
	Job   struct {
		File struct {
			Name string `json:"name"`
		} `json:"file"`
	} `json:"job"`
	Progress struct {
		Completion    *float64 `json:"completion"`
		PrintTime     *int     `json:"printTime"`
		PrintTimeLeft *int     `json:"printTimeLeft"`
	} `json:"progress"`
}

// classifyState maps OctoPrint's free-text state string to a normalized PState and
// whether the printer is actually connected (false → render as offline). Offline /
// closed is checked first so "Offline after error" classifies as offline.
func classifyState(text string) (adapter.PState, bool) {
	t := strings.ToLower(strings.TrimSpace(text))
	switch {
	case t == "":
		return adapter.Offline, false
	case strings.Contains(t, "offline"), strings.Contains(t, "closed"):
		return adapter.Offline, false
	case strings.Contains(t, "error"):
		return adapter.Errored, true
	case strings.Contains(t, "printing"), strings.Contains(t, "sending"), strings.Contains(t, "starting"):
		return adapter.Printing, true
	case strings.Contains(t, "pausing"), strings.Contains(t, "paused"):
		return adapter.Paused, true
	case strings.Contains(t, "cancelling"):
		return adapter.Cancelled, true
	default: // operational / ready / finishing / idle
		return adapter.Idle, true
	}
}

// refineFromFlags prefers the structured state flags from /api/printer (more
// reliable than the free-text string) when they are available.
func refineFromFlags(f octoFlags, fallback adapter.PState, connected bool) (adapter.PState, bool) {
	switch {
	case f.ClosedOrError && !f.Error:
		return adapter.Offline, false
	case f.Error:
		return adapter.Errored, true
	case f.Printing:
		return adapter.Printing, true
	case f.Pausing, f.Paused:
		return adapter.Paused, true
	case f.Cancelling:
		return adapter.Cancelled, true
	case f.Operational, f.Ready, f.Finishing:
		return adapter.Idle, true
	default:
		return fallback, connected
	}
}

func (p *Printer) getJSON(ctx context.Context, path string, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.httpBase+path, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("X-Api-Key", p.apiKey)
	resp, err := p.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("octoprint GET %s: HTTP %d", path, resp.StatusCode)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

func (p *Printer) postJSON(ctx context.Context, path string, body any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.httpBase+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", p.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("octoprint %s: HTTP %d", path, resp.StatusCode)
	}
	return nil
}

// ----- small helpers -----

func homeAxes(axes string) []string {
	axes = strings.ToLower(strings.TrimSpace(axes))
	if axes == "" {
		return []string{"x", "y", "z"}
	}
	var out []string
	for _, c := range axes {
		if c == 'x' || c == 'y' || c == 'z' {
			out = append(out, string(c))
		}
	}
	if len(out) == 0 {
		return []string{"x", "y", "z"}
	}
	return out
}
