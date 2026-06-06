// Package bambu implements adapter.Printer for Bambu Lab printers over local MQTT
// (the same protocol Bambu Studio uses in LAN mode). It connects over TLS to the
// printer on :8883 (user "bblp", password = the LAN Access Code), subscribes to
// device/<serial>/report for live status, and publishes commands to
// device/<serial>/request. Control requires the printer to be in LAN Mode /
// Developer Mode; when it is not, ControlBlocked is set and the UI renders the
// printer read-only (the "flippable" mechanism in adapter.Flags). All control
// safety gating mirrors the Moonraker adapter.
package bambu

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/MakerViking/TinkerView/internal/adapter"
	"github.com/MakerViking/TinkerView/internal/adapters/printerutil"
	"github.com/MakerViking/TinkerView/internal/bus"
)

const devModeMsg = "Enable LAN Mode / Developer Mode on the printer to allow control"

var errControlBlocked = errors.New("control is blocked — enable LAN Mode / Developer Mode on the printer")

type Printer struct {
	bus        *bus.Bus
	id         string
	name       string
	camera     string
	uiURL      string
	ip         string
	serial     string
	accessCode string
	model      string
	managed    bool
	macros     []string
	enclosed   bool // model has a chamber (show chamber temperature readout)

	minExtrudeTemp float64
	seq            int64 // command sequence id (atomic)

	mu             sync.Mutex
	client         mqtt.Client
	online         bool
	controlBlocked bool
	// report is the merged Bambu "print" object (partial deltas accumulate here).
	report map[string]json.RawMessage
}

func New(b *bus.Bus, id, name, ip, serial, accessCode, model, camera, uiURL string, managed bool, macros []string) *Printer {
	return &Printer{
		bus: b, id: id, name: name, camera: camera, uiURL: uiURL,
		ip: ip, serial: serial, accessCode: accessCode, model: model,
		managed: managed, macros: macros, enclosed: modelEnclosed(model),
		minExtrudeTemp: printerutil.DefaultMinExtrudeTemp,
		report:         map[string]json.RawMessage{},
	}
}

func (p *Printer) ID() string   { return p.id }
func (p *Printer) Kind() string { return "bambu" }

func (p *Printer) Snapshot() adapter.PrinterState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.toState()
}

// ----- lifecycle -----

func (p *Printer) Run(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("bambu adapter panic recovered", "printer", p.id, "recover", r)
		}
	}()
	p.publish(p.Snapshot()) // show the printer immediately (offline until first report)

	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("ssl://%s:8883", p.ip))
	opts.SetClientID("tinkerview-" + p.id)
	opts.SetUsername("bblp")
	opts.SetPassword(p.accessCode)
	// Bambu printers present a self-signed certificate; the access code is the auth.
	opts.SetTLSConfig(&tls.Config{InsecureSkipVerify: true}) //nolint:gosec // self-signed printer cert
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5 * time.Second)
	opts.SetKeepAlive(30 * time.Second)
	opts.SetOnConnectHandler(p.onConnect) // subscribe here so subs survive reconnects
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		slog.Warn("bambu: mqtt connection lost", "printer", p.id, "err", err)
		p.setOffline()
	})

	cl := mqtt.NewClient(opts)
	p.mu.Lock()
	p.client = cl
	p.mu.Unlock()
	defer cl.Disconnect(200)

	if t := cl.Connect(); t.Wait() && t.Error() != nil {
		slog.Error("bambu: initial mqtt connect failed (will retry)", "printer", p.id, "err", t.Error())
	}

	// Periodically re-request a full status (covers a missed report) and re-publish
	// so an idle printer's tile stays fresh and dropped bus deltas self-correct.
	pushall := time.NewTicker(30 * time.Second)
	republish := time.NewTicker(10 * time.Second)
	defer pushall.Stop()
	defer republish.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-pushall.C:
			p.requestPushAll()
		case <-republish.C:
			p.publish(p.Snapshot())
		}
	}
}

func (p *Printer) onConnect(_ mqtt.Client) {
	slog.Info("bambu: mqtt connected", "printer", p.id, "ip", p.ip)
	cl := p.getClient()
	if cl == nil {
		return
	}
	cl.Subscribe("device/"+p.serial+"/report", 0, p.onReport)
	go p.requestPushAll() // force a full status snapshot
}

// onReport merges a device/<serial>/report payload's "print" object into the cached
// state and republishes. Non-print categories (info/system/mc_print/...) are ignored.
func (p *Printer) onReport(_ mqtt.Client, m mqtt.Message) {
	var top struct {
		Print json.RawMessage `json:"print"`
	}
	if err := json.Unmarshal(m.Payload(), &top); err != nil || len(top.Print) == 0 {
		return
	}
	var delta map[string]json.RawMessage
	if err := json.Unmarshal(top.Print, &delta); err != nil {
		return
	}
	p.mu.Lock()
	for k, v := range delta {
		p.report[k] = v
	}
	p.online = true
	// We optimistically allow control once status reports flow. NOTE: newer Bambu
	// firmware (the ACS authorization system) accepts STATUS reads in LAN-Only mode
	// but REJECTS control commands (HMS_0500-0500-0001-0007) unless Developer Mode is
	// ALSO enabled — and that rejection is invisible on a fire-and-forget QoS-0
	// publish. If a printer silently ignores commands, the fix is enabling LAN +
	// Developer Mode on the printer screen. (Future: detect the ACS HMS code in the
	// report's "hms" array and flip controlBlocked + Message automatically.)
	p.controlBlocked = false
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

// toState maps the cached Bambu report to the normalized PrinterState. Caller holds p.mu.
func (p *Printer) toState() adapter.PrinterState {
	s := adapter.PrinterState{ID: p.id, Kind: "bambu", Name: p.name, Camera: p.camera, UIURL: p.uiURL, Online: p.online, Managed: p.managed}
	if !p.online {
		s.State = adapter.Offline
		if p.controlBlocked {
			s.Message = devModeMsg
		}
		return s
	}
	m := p.report
	s.Filename = firstNonEmpty(jstr(m, "subtask_name"), jstr(m, "gcode_file"))
	s.ProgressPct = jfloat(m, "mc_percent")
	s.EtaSec = jint(m, "mc_remaining_time") * 60 // Bambu reports minutes
	s.Nozzle = adapter.Heater{Actual: jfloat(m, "nozzle_temper"), Target: jfloat(m, "nozzle_target_temper")}
	s.Bed = adapter.Heater{Actual: jfloat(m, "bed_temper"), Target: jfloat(m, "bed_target_temper")}
	if ch := jfloat(m, "chamber_temper"); p.enclosed || ch > 0 {
		s.Chamber = &adapter.Heater{Actual: ch} // chamber is read-only on Bambu
	}
	s.Layer = jint(m, "layer_num")
	s.TotalLayers = jint(m, "total_layer_num")

	switch strings.ToUpper(jstr(m, "gcode_state")) {
	case "RUNNING", "PREPARE", "SLICING":
		s.State = adapter.Printing
		s.Flags = adapter.Flags{CanPause: true, CanCancel: true}
	case "PAUSE":
		s.State = adapter.Paused
		s.Flags = adapter.Flags{CanResume: true, CanCancel: true}
	case "FINISH":
		s.State = adapter.Complete
	case "FAILED":
		s.State = adapter.Errored
	default: // IDLE / unknown
		s.State = adapter.Idle
	}

	if p.controlBlocked {
		s.Flags = adapter.Flags{ControlBlocked: true}
		s.Message = devModeMsg
		return s
	}
	caps := adapter.Caps{SetTemps: true, Move: true}
	if len(p.macros) > 0 {
		caps.Macros = p.macros
	}
	s.Caps = &caps
	return s
}

// ----- control -----

func (p *Printer) Command(ctx context.Context, cmd adapter.PrinterCommand) error {
	if p.blocked() {
		return errControlBlocked
	}
	var c string
	switch cmd {
	case adapter.CmdPause:
		c = "pause"
	case adapter.CmdResume:
		c = "resume"
	case adapter.CmdCancel:
		c = "stop"
	default:
		return fmt.Errorf("unsupported command %q", cmd)
	}
	return p.request(ctx, map[string]any{"print": map[string]any{"command": c, "sequence_id": p.nextSeq()}})
}

func (p *Printer) SetTemp(ctx context.Context, heater string, target float64) error {
	if p.blocked() {
		return errControlBlocked
	}
	switch heater {
	case "bed":
		return p.gcodeLine(ctx, fmt.Sprintf("M140 S%.0f", target))
	case "nozzle", "extruder", "":
		return p.gcodeLine(ctx, fmt.Sprintf("M104 S%.0f", target))
	case "chamber":
		return fmt.Errorf("chamber heater is not settable on this printer")
	default:
		return fmt.Errorf("unsupported heater %q", heater)
	}
}

// Control dispatches an expanded action with the same safety gating as Moonraker,
// delivered as gcode_line commands over MQTT.
func (p *Printer) Control(ctx context.Context, a adapter.PrinterAction) error {
	if p.blocked() {
		return errControlBlocked
	}
	switch a.Op {
	case adapter.ActHome:
		if !p.isIdle() {
			return printerutil.ErrNotIdle
		}
		return p.gcodeLine(ctx, printerutil.HomeGcode(a.Axes))
	case adapter.ActMove:
		if !p.isIdle() {
			return printerutil.ErrNotIdle
		}
		ax := strings.ToUpper(a.Axis)
		if ax != "X" && ax != "Y" && ax != "Z" {
			return fmt.Errorf("invalid axis %q", a.Axis)
		}
		return p.gcodeLine(ctx, fmt.Sprintf("G91\nG0 %s%.3f F%.0f\nG90", ax, printerutil.ClampAbs(a.Dist, 100), printerutil.ClampFeed(a.Feed, a.Axis)))
	case adapter.ActExtrude:
		if !p.isIdle() {
			return printerutil.ErrNotIdle
		}
		if p.nozzleTemp() < p.minExtrudeTemp {
			return fmt.Errorf("nozzle below %.0f°C — heat it before extruding", p.minExtrudeTemp)
		}
		return p.gcodeLine(ctx, fmt.Sprintf("M83\nG1 E%.3f F%.0f", printerutil.ClampAbs(a.Dist, 100), printerutil.ClampFeed(a.Feed, "e")))
	case adapter.ActMacro:
		if !p.macroAllowed(a.Name) {
			return fmt.Errorf("macro %q is not in the allowed list", a.Name)
		}
		return p.gcodeLine(ctx, a.Name)
	default:
		return fmt.Errorf("unsupported action %q", a.Op)
	}
}

func (p *Printer) blocked() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.controlBlocked
}

func (p *Printer) isIdle() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	gs := strings.ToUpper(jstr(p.report, "gcode_state"))
	return gs != "RUNNING" && gs != "PAUSE" && gs != "PREPARE" && gs != "SLICING"
}

func (p *Printer) nozzleTemp() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return jfloat(p.report, "nozzle_temper")
}

// macroAllowed validates a macro name is in the curated allowlist (exact,
// case-insensitive) and contains no injection characters or extra tokens.
func (p *Printer) macroAllowed(name string) bool {
	return printerutil.MacroAllowed(name, p.macros)
}

// ----- MQTT plumbing -----

func (p *Printer) getClient() mqtt.Client {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.client
}

func (p *Printer) nextSeq() string {
	return strconv.FormatInt(atomic.AddInt64(&p.seq, 1), 10)
}

func (p *Printer) requestPushAll() {
	cl := p.getClient()
	if cl == nil || !cl.IsConnectionOpen() {
		return
	}
	cl.Publish("device/"+p.serial+"/request", 0, false, []byte(`{"pushing":{"sequence_id":"0","command":"pushall"}}`))
}

func (p *Printer) gcodeLine(ctx context.Context, script string) error {
	return p.request(ctx, map[string]any{"print": map[string]any{"command": "gcode_line", "param": script + "\n", "sequence_id": p.nextSeq()}})
}

func (p *Printer) request(ctx context.Context, payload any) error {
	cl := p.getClient()
	if cl == nil || !cl.IsConnectionOpen() {
		return errors.New("Bambu MQTT not connected")
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	tok := cl.Publish("device/"+p.serial+"/request", 0, false, b)
	select {
	case <-tok.Done():
		return tok.Error()
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		return errors.New("Bambu: publish timed out")
	}
}

// ----- small helpers -----

// modelEnclosed reports whether a Bambu model has a heated/measured chamber so the
// UI shows a chamber temperature readout (chamber is read-only on Bambu firmware).
func modelEnclosed(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(m, "x1") || strings.Contains(m, "p1s")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// jstr/jfloat/jint read a field from the merged report, tolerating either a JSON
// number or a quoted-string representation (Bambu firmwares vary).
func jstr(m map[string]json.RawMessage, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s
	}
	return strings.TrimSpace(string(v))
}

func jfloat(m map[string]json.RawMessage, key string) float64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	var f float64
	if json.Unmarshal(v, &f) == nil {
		return f
	}
	var s string
	if json.Unmarshal(v, &s) == nil {
		f, _ = strconv.ParseFloat(strings.TrimSpace(s), 64)
		return f
	}
	return 0
}

func jint(m map[string]json.RawMessage, key string) int {
	return int(jfloat(m, key))
}
