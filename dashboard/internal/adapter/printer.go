package adapter

import "context"

// PState is the normalized print state across all printer ecosystems.
type PState string

const (
	Offline   PState = "offline" // adapter can't reach the printer/host
	Idle      PState = "idle"    // standby/operational, ready
	Printing  PState = "printing"
	Paused    PState = "paused"
	Complete  PState = "complete"
	Errored   PState = "error"
	Cancelled PState = "cancelled"
)

// Heater is an actual/target temperature pair in degrees Celsius.
type Heater struct {
	Actual float64 `json:"actual"`
	Target float64 `json:"target"`
}

// Flags advertise which controls the UI may show. ControlBlocked is set when
// control is structurally unavailable (e.g. a Bambu printer not in LAN+Developer
// mode); Message on PrinterState then explains why. This is the "flippable"
// mechanism: a read-only printer becomes controllable with no code change once
// the precondition is met.
type Flags struct {
	CanPause       bool `json:"canPause"`
	CanResume      bool `json:"canResume"`
	CanCancel      bool `json:"canCancel"`
	ControlBlocked bool `json:"controlBlocked"`
}

// Caps advertises which expanded controls + info this printer supports, so the UI
// shows only what it can do. Nil on adapters that support none. Discovered names
// (fans, chamber heater, sensor) and the curated macro list ride along so the UI
// can render exactly the right buttons.
type Caps struct {
	SetTemps        bool     `json:"setTemps"`
	Chamber         bool     `json:"chamber"`        // a chamber heater was discovered
	Fans            []FanCap `json:"fans,omitempty"` // controllable fans (discovered)
	FilamentSensor  bool     `json:"filamentSensor"` // a filament_switch_sensor exists
	SpeedFlow       bool     `json:"speedFlow"`      // gcode_move present (M220/M221)
	Babystep        bool     `json:"babystep"`
	Move            bool     `json:"move"` // toolhead present (home/jog/extrude)
	EmergencyStop   bool     `json:"emergencyStop"`
	FirmwareRestart bool     `json:"firmwareRestart"`
	FileBrowser     bool     `json:"fileBrowser"` // Moonraker proxy available
	History         bool     `json:"history"`
	Macros          []string `json:"macros,omitempty"` // curated allowlist for the UI
}

// FanCap is one controllable fan. Object is the raw Klipper object used to address
// it (e.g. "fan_generic part_fan"); Key is a stable UI id; Kind drives the label.
type FanCap struct {
	Key    string `json:"key"`    // "part" | "chamber" | derived name
	Object string `json:"object"` // "fan" | "chamber_fan" | "fan_generic <name>"
	Kind   string `json:"kind"`   // "part" | "chamber" | "generic"
}

// Fan is live state for one fan (0..1 speed).
type Fan struct {
	Key   string  `json:"key"`
	Speed float64 `json:"speed"`         // 0..1
	RPM   *int    `json:"rpm,omitempty"` // if the fan reports tach
}

// FilamentSensor is the live filament_switch_sensor state.
type FilamentSensor struct {
	Name     string `json:"name"`
	Detected bool   `json:"detected"`
	Enabled  bool   `json:"enabled"`
}

// Motion is toolhead position, homed axes, and the live speed/flow factors.
type Motion struct {
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	Z           float64 `json:"z"`
	HomedAxes   string  `json:"homedAxes"`   // "xyz" | "xy" | "" (raw Klipper)
	SpeedFactor float64 `json:"speedFactor"` // M220 value as percent (100 == 1.0)
	FlowFactor  float64 `json:"flowFactor"`  // M221 value as percent
	ZOffset     float64 `json:"zOffset"`     // homing_origin Z (babystep accumulation)
}

// HostLoad is the printer host's load (Moonraker proc_stats / Klipper system_stats).
type HostLoad struct {
	CPUPct  float64 `json:"cpuPct"`  // host CPU usage %
	MemPct  float64 `json:"memPct"`  // host memory usage %
	MCULoad float64 `json:"mcuLoad"` // Klipper sysload
}

// PrinterAction is a tagged control request routed to the owning adapter via
// Control. Op selects the variant; the remaining fields carry that variant's args.
// Validation (idle-gating, macro allowlist, min-extrude-temp, clamping) is the
// adapter's responsibility — it is the only layer that knows the live state.
type PrinterAction struct {
	Op   string  `json:"op"`
	Name string  `json:"name,omitempty"` // fan object | macro name | file path
	Pct  float64 `json:"pct,omitempty"`  // fan/speed/flow percent
	Axis string  `json:"axis,omitempty"` // "x"|"y"|"z" for move
	Dist float64 `json:"dist,omitempty"` // mm (signed) for move/extrude/babystep
	Feed float64 `json:"feed,omitempty"` // mm/min (adapter clamps)
	Axes string  `json:"axes,omitempty"` // "xyz"|"x"... for home ("" == all)
}

// PrinterAction.Op values.
const (
	ActFan             = "fan"
	ActSpeed           = "speed"
	ActFlow            = "flow"
	ActBabystep        = "babystep"
	ActEmergencyStop   = "estop"
	ActFirmwareRestart = "firmware_restart"
	ActKlipperRestart  = "restart"
	ActHome            = "home"
	ActMove            = "move"
	ActExtrude         = "extrude"
	ActMacro           = "macro"
	ActPrintFile       = "print_file"
	ActReprint         = "reprint"
)

// PrinterState is the single normalized snapshot the frontend renders, identical
// for Moonraker / OctoPrint / Bambu. Mirrored in web/src/types/ws.ts.
type PrinterState struct {
	ID          string  `json:"id"`
	Kind        string  `json:"kind"` // moonraker | octoprint | bambu
	Name        string  `json:"name"`
	Online      bool    `json:"online"` // adapter reached the printer/host
	State       PState  `json:"state"`
	Filename    string  `json:"filename"`
	ProgressPct float64 `json:"progressPct"` // 0..100
	EtaSec      int     `json:"etaSec"`      // remaining seconds
	ElapsedSec  int     `json:"elapsedSec"`
	Nozzle      Heater  `json:"nozzle"`
	Bed         Heater  `json:"bed"`
	Chamber     *Heater `json:"chamber,omitempty"` // enclosed printers only
	Layer       int     `json:"layer"`
	TotalLayers int     `json:"totalLayers"`
	Flags       Flags   `json:"flags"`
	Message     string  `json:"message,omitempty"` // error text / "Developer Mode required"
	Camera      string  `json:"camera,omitempty"`  // go2rtc stream name, if any
	UIURL       string  `json:"uiUrl,omitempty"`   // printer's own web UI (Fluidd/Mainsail/OctoPrint)
	// Managed is true when the dashboard owns this printer's config (added via the
	// UI / device store), so the UI may offer edit/remove. False for printers that
	// only exist in the read-only config.yaml seed.
	Managed bool `json:"managed"`
	// Deleted signals the printer no longer exists (the hub removes it from cache
	// and tells clients to drop the tile). Mirrors CameraState.Deleted; bus-only,
	// never serialized — the hub translates it to a "printer.delete" message.
	Deleted bool `json:"-"`
	// Expanded control capabilities + live info (Moonraker). nil/empty on adapters
	// that don't support them, so the UI hides the corresponding controls.
	Caps           *Caps           `json:"caps,omitempty"`
	Fans           []Fan           `json:"fans,omitempty"`
	FilamentSensor *FilamentSensor `json:"filamentSensor,omitempty"`
	Motion         *Motion         `json:"motion,omitempty"`
	HostLoad       *HostLoad       `json:"hostLoad,omitempty"`
}

// PrinterCommand is a normalized control request routed to the owning adapter.
type PrinterCommand string

const (
	CmdPause  PrinterCommand = "pause"
	CmdResume PrinterCommand = "resume"
	CmdCancel PrinterCommand = "cancel"
)

// Printer is implemented by each ecosystem adapter. Run owns the persistent
// connection + reconnect loop and Publishes normalized PrinterState events to
// the bus; it must never panic the process (wrap work in recover) and never
// touch websockets directly.
type Printer interface {
	ID() string
	Kind() string
	// Snapshot returns the authoritative cached state for the WS snapshot.
	Snapshot() PrinterState
	// Run blocks until ctx is cancelled, maintaining the connection and
	// publishing PrinterState deltas to the bus.
	Run(ctx context.Context)
	// Command issues a normalized control action. Returns an error (surfaced to
	// the user) if blocked or the printer rejects it.
	Command(ctx context.Context, cmd PrinterCommand) error
	// SetTemp best-effort sets a heater target ("nozzle"|"bed"|"chamber").
	SetTemp(ctx context.Context, heater string, target float64) error
	// Control issues an expanded control action (fan/speed/flow/babystep/move/
	// macro/print/...). Adapters that don't support an action return an error.
	Control(ctx context.Context, a PrinterAction) error
}
