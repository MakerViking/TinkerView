// Package moonraker implements adapter.Printer for Klipper printers via the
// Moonraker API (Mainsail/Fluidd). Live state comes over the Moonraker WebSocket
// JSON-RPC (printer.objects.subscribe); control uses simple HTTP POSTs. At session
// start it discovers the printer's objects (fan/chamber/sensor names vary per
// printer) so it subscribes to the right ones and advertises Caps to the UI.
package moonraker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/MakerViking/TinkerView/internal/adapter"
	"github.com/MakerViking/TinkerView/internal/adapters/printerutil"
	"github.com/MakerViking/TinkerView/internal/bus"
)

// defaultMacros is the safe set advertised when a printer has no curated macros
// configured; intersected with the printer's actually-available macros.
var defaultMacros = []string{"BED_MESH_CALIBRATE", "CLEAN_NOZZLE", "CLEAR_NOZZLE", "LOAD_FILAMENT", "UNLOAD_FILAMENT", "M600", "PARK"}

type Printer struct {
	bus     *bus.Bus
	id      string
	name    string
	camera  string
	httpURL string // http://ip (Fluidd/Mainsail proxy or :7125)
	wsURL   string // ws://ip/websocket
	uiURL   string // printer's web UI to open on click
	apiKey  string
	managed bool     // dashboard owns this printer's config (UI can edit/remove)
	macros  []string // curated macro allowlist (from config); empty = default set
	client  *http.Client

	mu             sync.Mutex
	st             klipperState
	online         bool
	minExtrudeTemp float64

	// discovered at session start (object names vary per printer)
	caps           adapter.Caps
	chamberHeater  string // full object name, e.g. "heater_generic chamber"
	filamentSensor string // full object name
	fans           []adapter.FanCap

	// dynamic object state (names not known at compile time)
	dynChamber *heaterVal
	dynFans    map[string]fanVal // keyed by FanCap.Key
	dynSensor  *sensorVal
	hostLoad   adapter.HostLoad
}

func New(b *bus.Bus, id, name, baseURL, apiKey, camera, uiURL string, managed bool, macros []string) *Printer {
	base := strings.TrimRight(baseURL, "/")
	// http://x -> ws://x , https://x -> wss://x
	ws := "ws" + strings.TrimPrefix(base, "http") + "/websocket"
	if uiURL == "" {
		uiURL = base
	}
	return &Printer{
		bus: b, id: id, name: name, camera: camera,
		httpURL: base, wsURL: ws, uiURL: uiURL, apiKey: apiKey, managed: managed, macros: macros,
		client:         &http.Client{Timeout: 10 * time.Second},
		minExtrudeTemp: printerutil.DefaultMinExtrudeTemp,
		dynFans:        map[string]fanVal{},
	}
}

func (p *Printer) ID() string   { return p.id }
func (p *Printer) Kind() string { return "moonraker" }

func (p *Printer) Snapshot() adapter.PrinterState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.toState()
}

// klipperState mirrors the fixed (always-present) subscribed objects. Partial
// deltas are merged by json.Unmarshal into this existing struct. Dynamically-named
// objects (chamber heater, fans, filament sensor) are decoded separately.
type klipperState struct {
	PrintStats struct {
		State         string  `json:"state"`
		Filename      string  `json:"filename"`
		PrintDuration float64 `json:"print_duration"`
		TotalDuration float64 `json:"total_duration"`
		Info          struct {
			TotalLayer   *int `json:"total_layer"`
			CurrentLayer *int `json:"current_layer"`
		} `json:"info"`
	} `json:"print_stats"`
	VirtualSdcard struct {
		Progress float64 `json:"progress"`
	} `json:"virtual_sdcard"`
	DisplayStatus struct {
		Progress float64 `json:"progress"`
	} `json:"display_status"`
	Extruder struct {
		Temperature float64 `json:"temperature"`
		Target      float64 `json:"target"`
	} `json:"extruder"`
	HeaterBed struct {
		Temperature float64 `json:"temperature"`
		Target      float64 `json:"target"`
	} `json:"heater_bed"`
	GcodeMove struct {
		SpeedFactor   float64    `json:"speed_factor"`
		ExtrudeFactor float64    `json:"extrude_factor"`
		HomingOrigin  [4]float64 `json:"homing_origin"`
		GcodePosition [4]float64 `json:"gcode_position"`
	} `json:"gcode_move"`
	Toolhead struct {
		Position  [4]float64 `json:"position"`
		HomedAxes string     `json:"homed_axes"`
	} `json:"toolhead"`
	SystemStats struct {
		Sysload float64 `json:"sysload"`
	} `json:"system_stats"`
	IdleTimeout struct {
		State string `json:"state"` // "Idle" | "Printing" | "Ready"
	} `json:"idle_timeout"`
}

type heaterVal struct {
	Temperature float64 `json:"temperature"`
	Target      float64 `json:"target"`
}
type fanVal struct {
	Speed float64 `json:"speed"`
	RPM   *int    `json:"rpm"`
}
type sensorVal struct {
	FilamentDetected bool `json:"filament_detected"`
	Enabled          bool `json:"enabled"`
}

// toState maps the cached Klipper state to the normalized PrinterState. Caller holds p.mu.
func (p *Printer) toState() adapter.PrinterState {
	s := adapter.PrinterState{ID: p.id, Kind: "moonraker", Name: p.name, Camera: p.camera, UIURL: p.uiURL, Online: p.online, Managed: p.managed}
	if !p.online {
		s.State = adapter.Offline
		return s
	}
	ks := p.st
	s.Filename = ks.PrintStats.Filename
	s.ElapsedSec = int(ks.PrintStats.PrintDuration)
	prog := ks.VirtualSdcard.Progress
	if ks.DisplayStatus.Progress > 0 { // M73 slicer progress is more accurate when present
		prog = ks.DisplayStatus.Progress
	}
	s.ProgressPct = prog * 100
	s.Nozzle = adapter.Heater{Actual: ks.Extruder.Temperature, Target: ks.Extruder.Target}
	s.Bed = adapter.Heater{Actual: ks.HeaterBed.Temperature, Target: ks.HeaterBed.Target}
	if ks.PrintStats.Info.CurrentLayer != nil {
		s.Layer = *ks.PrintStats.Info.CurrentLayer
	}
	if ks.PrintStats.Info.TotalLayer != nil {
		s.TotalLayers = *ks.PrintStats.Info.TotalLayer
	}
	switch ks.PrintStats.State {
	case "printing":
		s.State = adapter.Printing
		s.Flags = adapter.Flags{CanPause: true, CanCancel: true}
		if prog > 0 && prog <= 1.0 && ks.PrintStats.PrintDuration > 0 {
			s.EtaSec = int(ks.PrintStats.PrintDuration * (1/prog - 1))
		}
	case "paused":
		s.State = adapter.Paused
		s.Flags = adapter.Flags{CanResume: true, CanCancel: true}
	case "complete":
		s.State = adapter.Complete
	case "cancelled":
		s.State = adapter.Cancelled
	case "error":
		s.State = adapter.Errored
	default: // "standby"
		s.State = adapter.Idle
	}

	// --- expanded capabilities + live info ---
	caps := p.caps
	s.Caps = &caps
	if p.dynChamber != nil {
		s.Chamber = &adapter.Heater{Actual: p.dynChamber.Temperature, Target: p.dynChamber.Target}
	}
	for _, f := range p.fans {
		fv := p.dynFans[f.Key]
		// Clone RPM: the published *int must not alias p.dynFans (which applyStatus
		// mutates in place on a fan delta), else the hub re-marshals it concurrently.
		var rpm *int
		if fv.RPM != nil {
			v := *fv.RPM
			rpm = &v
		}
		s.Fans = append(s.Fans, adapter.Fan{Key: f.Key, Speed: fv.Speed, RPM: rpm})
	}
	if p.dynSensor != nil {
		s.FilamentSensor = &adapter.FilamentSensor{Name: bareName(p.filamentSensor), Detected: p.dynSensor.FilamentDetected, Enabled: p.dynSensor.Enabled}
	}
	s.Motion = &adapter.Motion{
		X: ks.Toolhead.Position[0], Y: ks.Toolhead.Position[1], Z: ks.Toolhead.Position[2],
		HomedAxes:   ks.Toolhead.HomedAxes,
		SpeedFactor: ks.GcodeMove.SpeedFactor * 100,
		FlowFactor:  ks.GcodeMove.ExtrudeFactor * 100,
		ZOffset:     ks.GcodeMove.HomingOrigin[2],
	}
	hl := p.hostLoad
	hl.MCULoad = ks.SystemStats.Sysload
	s.HostLoad = &hl
	return s
}

func (p *Printer) Run(ctx context.Context) {
	p.publish(p.Snapshot()) // show the printer immediately (offline until connected)

	// Periodic re-publish (covers idle printers) + a slower host-load poll.
	go func() {
		republish := time.NewTicker(10 * time.Second)
		host := time.NewTicker(5 * time.Second)
		defer republish.Stop()
		defer host.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-republish.C:
				p.publish(p.Snapshot())
			case <-host.C:
				p.fetchProcStats(ctx)
			}
		}
	}()

	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		err := p.session(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			slog.Warn("moonraker session ended", "printer", p.id, "err", err)
		}
		p.setOffline()
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

func (p *Printer) session(ctx context.Context) error {
	dctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	header := http.Header{}
	if p.apiKey != "" {
		header.Set("X-Api-Key", p.apiKey)
	}
	conn, _, err := websocket.Dial(dctx, p.wsURL, &websocket.DialOptions{HTTPHeader: header})
	cancel()
	if err != nil {
		return err
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	conn.SetReadLimit(1 << 20)

	p.discover(ctx) // HTTP: learn object names + capabilities before subscribing
	if err := p.subscribe(ctx, conn); err != nil {
		return err
	}
	for {
		var msg rpcMsg
		if err := wsjson.Read(ctx, conn, &msg); err != nil {
			return err
		}
		p.handle(ctx, conn, &msg)
	}
}

// discover queries the live object list (HTTP, no WS interleaving) so we subscribe
// to the right dynamically-named objects and set Caps. Best-effort: on failure we
// fall back to the fixed object set with minimal caps.
func (p *Printer) discover(ctx context.Context) {
	objs, err := p.listObjects(ctx)
	if err != nil {
		slog.Debug("moonraker discover failed", "printer", p.id, "err", err)
		return
	}
	caps := adapter.Caps{SetTemps: true, EmergencyStop: true, FirmwareRestart: true, FileBrowser: true, History: true}
	var chamber, sensor string
	var fans []adapter.FanCap
	for _, o := range objs {
		switch {
		case o == "fan":
			fans = append(fans, adapter.FanCap{Key: "part", Object: "fan", Kind: "part"})
		case strings.HasPrefix(o, "fan_generic "):
			n := bareName(o)
			fans = append(fans, adapter.FanCap{Key: n, Object: o, Kind: "generic"})
		case strings.HasPrefix(o, "heater_generic "):
			if chamber == "" || strings.Contains(strings.ToLower(o), "chamber") {
				chamber = o
			}
		case strings.HasPrefix(o, "filament_switch_sensor "):
			if sensor == "" {
				sensor = o
			}
		case o == "gcode_move":
			caps.SpeedFlow = true
			caps.Babystep = true
		case o == "toolhead":
			caps.Move = true
		}
	}
	caps.Chamber = chamber != ""
	caps.FilamentSensor = sensor != ""
	caps.Fans = fans
	caps.Macros = p.curatedMacros(objs)

	p.mu.Lock()
	p.caps = caps
	p.chamberHeater = chamber
	p.filamentSensor = sensor
	p.fans = fans
	// Drop dynamic state for objects that no longer exist (e.g. a chamber heater
	// removed across a Klipper reconfigure), else toState would emit a frozen ghost.
	if chamber == "" {
		p.dynChamber = nil
	}
	if sensor == "" {
		p.dynSensor = nil
	}
	p.mu.Unlock()
}

// curatedMacros returns the macro allowlist advertised to the UI. A configured
// list is trusted verbatim (the operator picked valid commands, e.g. T0..T3).
// Otherwise a safe default set is intersected with the printer's gcode_macros.
func (p *Printer) curatedMacros(objs []string) []string {
	if len(p.macros) > 0 {
		return p.macros
	}
	avail := map[string]bool{}
	for _, o := range objs {
		if n, ok := strings.CutPrefix(o, "gcode_macro "); ok {
			avail[strings.ToUpper(n)] = true
		}
	}
	var out []string
	for _, m := range defaultMacros {
		if avail[strings.ToUpper(m)] {
			out = append(out, m)
		}
	}
	return out
}

func (p *Printer) subscribe(ctx context.Context, conn *websocket.Conn) error {
	objects := map[string]any{
		"print_stats": nil, "virtual_sdcard": nil, "display_status": nil,
		"extruder": nil, "heater_bed": nil,
		"gcode_move": nil, "toolhead": nil, "system_stats": nil, "idle_timeout": nil,
	}
	p.mu.Lock()
	if p.chamberHeater != "" {
		objects[p.chamberHeater] = nil
	}
	for _, f := range p.fans {
		objects[f.Object] = nil
	}
	if p.filamentSensor != "" {
		objects[p.filamentSensor] = nil
	}
	p.mu.Unlock()

	req := map[string]any{
		"jsonrpc": "2.0",
		"method":  "printer.objects.subscribe",
		"params":  map[string]any{"objects": objects},
		"id":      1,
	}
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return wsjson.Write(wctx, conn, req)
}

type rpcMsg struct {
	ID     *int            `json:"id"`
	Method string          `json:"method"`
	Result json.RawMessage `json:"result"`
	Params json.RawMessage `json:"params"`
}

func (p *Printer) handle(ctx context.Context, conn *websocket.Conn, m *rpcMsg) {
	switch {
	case m.ID != nil && len(m.Result) > 0:
		// subscribe response: {"eventtime":..,"status":{...full...}}
		var r struct {
			Status json.RawMessage `json:"status"`
		}
		if json.Unmarshal(m.Result, &r) == nil && len(r.Status) > 0 {
			p.applyStatus(r.Status)
		}
	case m.Method == "notify_status_update":
		// params is the positional array [statusMap, eventtime]
		var arr []json.RawMessage
		if json.Unmarshal(m.Params, &arr) == nil && len(arr) > 0 {
			p.applyStatus(arr[0])
		}
	case m.Method == "notify_klippy_ready":
		p.discover(ctx)            // re-learn objects after a Klipper restart
		_ = p.subscribe(ctx, conn) // re-seed the subscription
	case m.Method == "notify_klippy_shutdown", m.Method == "notify_klippy_disconnected":
		p.setOffline()
	}
}

func (p *Printer) applyStatus(raw json.RawMessage) {
	p.mu.Lock()
	if err := json.Unmarshal(raw, &p.st); err != nil { // merge fixed objects
		p.mu.Unlock()
		slog.Warn("moonraker: status decode failed", "printer", p.id, "err", err)
		return
	}
	// Merge dynamically-named objects (names not known at compile time).
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) == nil {
		if p.chamberHeater != "" {
			if v, ok := m[p.chamberHeater]; ok {
				if p.dynChamber == nil {
					p.dynChamber = &heaterVal{}
				}
				_ = json.Unmarshal(v, p.dynChamber)
			}
		}
		for _, f := range p.fans {
			if v, ok := m[f.Object]; ok {
				fv := p.dynFans[f.Key]
				_ = json.Unmarshal(v, &fv)
				p.dynFans[f.Key] = fv
			}
		}
		if p.filamentSensor != "" {
			if v, ok := m[p.filamentSensor]; ok {
				if p.dynSensor == nil {
					p.dynSensor = &sensorVal{}
				}
				_ = json.Unmarshal(v, p.dynSensor)
			}
		}
	}
	p.online = true
	st := p.toState()
	p.mu.Unlock()
	p.publish(st)
}

func (p *Printer) setOffline() {
	p.mu.Lock()
	p.online = false
	st := p.toState()
	p.mu.Unlock()
	p.publish(st)
}

func (p *Printer) publish(st adapter.PrinterState) {
	p.bus.Publish(bus.Event{Source: bus.SourcePrinter, Kind: bus.KindPrinterState, ID: p.id, Ts: time.Now(), Payload: st})
}

// fetchProcStats polls the host load (Moonraker /machine/proc_stats) and, if the
// printer is online, republishes so the UI's host-load readout stays fresh.
func (p *Printer) fetchProcStats(ctx context.Context) {
	var r struct {
		Result struct {
			SystemCPUUsage struct {
				CPU float64 `json:"cpu"`
			} `json:"system_cpu_usage"`
			SystemMemory struct {
				Total int64 `json:"total"`
				Used  int64 `json:"used"`
			} `json:"system_memory"`
		} `json:"result"`
	}
	if err := p.get(ctx, "/machine/proc_stats", &r); err != nil {
		return
	}
	p.mu.Lock()
	p.hostLoad.CPUPct = r.Result.SystemCPUUsage.CPU
	if r.Result.SystemMemory.Total > 0 {
		p.hostLoad.MemPct = float64(r.Result.SystemMemory.Used) / float64(r.Result.SystemMemory.Total) * 100
	}
	online := p.online
	st := p.toState()
	p.mu.Unlock()
	if online {
		p.publish(st)
	}
}

// ----- control (HTTP) -----

func (p *Printer) Command(ctx context.Context, cmd adapter.PrinterCommand) error {
	switch cmd {
	case adapter.CmdPause:
		return p.post(ctx, "/printer/print/pause")
	case adapter.CmdResume:
		return p.post(ctx, "/printer/print/resume")
	case adapter.CmdCancel:
		return p.post(ctx, "/printer/print/cancel")
	default:
		return fmt.Errorf("unsupported command %q", cmd)
	}
}

func (p *Printer) SetTemp(ctx context.Context, heater string, target float64) error {
	var gcode string
	switch heater {
	case "bed":
		gcode = fmt.Sprintf("M140 S%.0f", target)
	case "nozzle", "extruder", "":
		gcode = fmt.Sprintf("M104 S%.0f", target)
	case "chamber":
		p.mu.Lock()
		name := p.chamberHeater
		p.mu.Unlock()
		if name == "" {
			return fmt.Errorf("this printer has no chamber heater")
		}
		gcode = fmt.Sprintf("SET_HEATER_TEMPERATURE HEATER=%s TARGET=%.0f", bareName(name), target)
	default:
		return fmt.Errorf("unsupported heater %q", heater)
	}
	return p.gcode(ctx, gcode)
}

// Control dispatches an expanded action with all the safety gating.
func (p *Printer) Control(ctx context.Context, a adapter.PrinterAction) error {
	switch a.Op {
	case adapter.ActFan:
		fc, ok := p.fanByKey(a.Name)
		if !ok {
			return fmt.Errorf("unknown fan %q", a.Name)
		}
		pct := printerutil.Clamp(a.Pct, 0, 100)
		if fc.Kind == "part" {
			return p.gcode(ctx, fmt.Sprintf("M106 S%.0f", pct/100*255))
		}
		return p.gcode(ctx, fmt.Sprintf("SET_FAN_SPEED FAN=%s SPEED=%.2f", fc.Key, pct/100))
	case adapter.ActSpeed:
		return p.gcode(ctx, fmt.Sprintf("M220 S%.0f", printerutil.Clamp(a.Pct, 10, 300)))
	case adapter.ActFlow:
		return p.gcode(ctx, fmt.Sprintf("M221 S%.0f", printerutil.Clamp(a.Pct, 50, 200)))
	case adapter.ActBabystep:
		return p.gcode(ctx, fmt.Sprintf("SET_GCODE_OFFSET Z_ADJUST=%.3f MOVE=1", printerutil.ClampAbs(a.Dist, 0.5)))
	case adapter.ActEmergencyStop:
		return p.post(ctx, "/printer/emergency_stop")
	case adapter.ActFirmwareRestart:
		return p.post(ctx, "/printer/firmware_restart")
	case adapter.ActKlipperRestart:
		return p.post(ctx, "/printer/restart")
	case adapter.ActHome:
		if !p.isIdle() {
			return printerutil.ErrNotIdle
		}
		return p.gcode(ctx, printerutil.HomeGcode(a.Axes))
	case adapter.ActMove:
		if !p.isIdle() {
			return printerutil.ErrNotIdle
		}
		ax := strings.ToUpper(a.Axis)
		if ax != "X" && ax != "Y" && ax != "Z" {
			return fmt.Errorf("invalid axis %q", a.Axis)
		}
		return p.gcode(ctx, fmt.Sprintf("G91\nG0 %s%.3f F%.0f\nG90", ax, printerutil.ClampAbs(a.Dist, 100), printerutil.ClampFeed(a.Feed, a.Axis)))
	case adapter.ActExtrude:
		if !p.isIdle() {
			return printerutil.ErrNotIdle
		}
		if p.nozzleTemp() < p.minExtrudeTemp {
			return fmt.Errorf("nozzle below %.0f°C — heat it before extruding", p.minExtrudeTemp)
		}
		return p.gcode(ctx, fmt.Sprintf("M83\nG1 E%.3f F%.0f", printerutil.ClampAbs(a.Dist, 100), printerutil.ClampFeed(a.Feed, "e")))
	case adapter.ActMacro:
		if !p.macroAllowed(a.Name) {
			return fmt.Errorf("macro %q is not in the allowed list", a.Name)
		}
		return p.gcode(ctx, a.Name)
	case adapter.ActPrintFile:
		if strings.ContainsAny(a.Name, "\n\r") {
			return fmt.Errorf("invalid filename")
		}
		return p.post(ctx, "/printer/print/start?filename="+url.QueryEscape(a.Name))
	case adapter.ActReprint:
		last := p.lastFilename()
		if last == "" {
			return fmt.Errorf("no previous print to reprint")
		}
		return p.post(ctx, "/printer/print/start?filename="+url.QueryEscape(last))
	default:
		return fmt.Errorf("unsupported action %q", a.Op)
	}
}

func (p *Printer) gcode(ctx context.Context, script string) error {
	return p.post(ctx, "/printer/gcode/script?script="+url.QueryEscape(script))
}

func (p *Printer) isIdle() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	st := p.st.PrintStats.State
	return st != "printing" && st != "paused"
}

func (p *Printer) nozzleTemp() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.st.Extruder.Temperature
}

func (p *Printer) lastFilename() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.st.PrintStats.Filename
}

func (p *Printer) fanByKey(key string) (adapter.FanCap, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, f := range p.fans {
		if f.Key == key {
			return f, true
		}
	}
	return adapter.FanCap{}, false
}

// macroAllowed validates a macro name is in the curated allowlist (exact,
// case-insensitive) and contains no injection characters or extra tokens.
func (p *Printer) macroAllowed(name string) bool {
	p.mu.Lock()
	allowed := p.caps.Macros
	p.mu.Unlock()
	return printerutil.MacroAllowed(name, allowed)
}

func (p *Printer) post(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.httpURL+path, nil)
	if err != nil {
		return err
	}
	if p.apiKey != "" {
		req.Header.Set("X-Api-Key", p.apiKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("moonraker %s: HTTP %d", path, resp.StatusCode)
	}
	return nil
}

func (p *Printer) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.httpURL+path, nil)
	if err != nil {
		return err
	}
	if p.apiKey != "" {
		req.Header.Set("X-Api-Key", p.apiKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("moonraker GET %s: HTTP %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (p *Printer) listObjects(ctx context.Context) ([]string, error) {
	var r struct {
		Result struct {
			Objects []string `json:"objects"`
		} `json:"result"`
	}
	if err := p.get(ctx, "/printer/objects/list", &r); err != nil {
		return nil, err
	}
	return r.Result.Objects, nil
}

// ----- small helpers -----

func bareName(full string) string {
	if i := strings.IndexByte(full, ' '); i >= 0 {
		return full[i+1:]
	}
	return full
}
