package bambu

import (
	"encoding/json"
	"testing"

	"github.com/MakerViking/TinkerView/internal/adapter"
)

func rep(pairs map[string]string) map[string]json.RawMessage {
	m := map[string]json.RawMessage{}
	for k, v := range pairs {
		m[k] = json.RawMessage(v)
	}
	return m
}

func TestToStateRunning(t *testing.T) {
	p := &Printer{id: "x1", name: "X1", online: true, enclosed: true, report: rep(map[string]string{
		"gcode_state":          `"RUNNING"`,
		"mc_percent":           `42`,
		"mc_remaining_time":    `30`,
		"nozzle_temper":        `211.5`,
		"nozzle_target_temper": `220`,
		"bed_temper":           `60`,
		"layer_num":            `12`,
		"total_layer_num":      `"100"`, // string form tolerated
		"subtask_name":         `"bench.3mf"`,
		"chamber_temper":       `35`,
	})}
	s := p.toState()
	if s.State != adapter.Printing {
		t.Errorf("state = %s, want printing", s.State)
	}
	if s.ProgressPct != 42 {
		t.Errorf("progress = %v", s.ProgressPct)
	}
	if s.EtaSec != 30*60 {
		t.Errorf("eta = %v, want %v", s.EtaSec, 30*60)
	}
	if s.Nozzle.Actual != 211.5 || s.Nozzle.Target != 220 {
		t.Errorf("nozzle = %+v", s.Nozzle)
	}
	if s.TotalLayers != 100 {
		t.Errorf("total layers = %v, want 100 (string tolerated)", s.TotalLayers)
	}
	if s.Chamber == nil || s.Chamber.Actual != 35 {
		t.Errorf("chamber = %+v", s.Chamber)
	}
	if s.Filename != "bench.3mf" {
		t.Errorf("filename = %q", s.Filename)
	}
	if s.Caps == nil || !s.Caps.SetTemps || !s.Caps.Move {
		t.Errorf("caps = %+v", s.Caps)
	}
	if !s.Flags.CanPause || !s.Flags.CanCancel {
		t.Errorf("flags = %+v", s.Flags)
	}
}

func TestToStateGcodeStates(t *testing.T) {
	for native, want := range map[string]adapter.PState{
		"PAUSE":  adapter.Paused,
		"FINISH": adapter.Complete,
		"FAILED": adapter.Errored,
		"IDLE":   adapter.Idle,
	} {
		p := &Printer{online: true, report: rep(map[string]string{"gcode_state": `"` + native + `"`})}
		if s := p.toState(); s.State != want {
			t.Errorf("gcode_state %q → %s, want %s", native, s.State, want)
		}
	}
}

func TestOfflineHasNoCaps(t *testing.T) {
	p := &Printer{online: false, report: rep(map[string]string{"gcode_state": `"IDLE"`})}
	if s := p.toState(); s.State != adapter.Offline || s.Caps != nil {
		t.Errorf("offline printer: state=%s caps=%+v", s.State, s.Caps)
	}
}

func TestControlBlockedHidesCaps(t *testing.T) {
	p := &Printer{online: true, controlBlocked: true, report: rep(map[string]string{"gcode_state": `"IDLE"`})}
	s := p.toState()
	if s.Caps != nil {
		t.Errorf("blocked printer should advertise no caps, got %+v", s.Caps)
	}
	if !s.Flags.ControlBlocked || s.Message == "" {
		t.Errorf("expected controlBlocked flag + message, got %+v / %q", s.Flags, s.Message)
	}
}

func TestIsIdle(t *testing.T) {
	for native, idle := range map[string]bool{
		"IDLE": true, "FINISH": true, "FAILED": true,
		"RUNNING": false, "PAUSE": false, "PREPARE": false,
	} {
		p := &Printer{report: rep(map[string]string{"gcode_state": `"` + native + `"`})}
		if got := p.isIdle(); got != idle {
			t.Errorf("isIdle(%q) = %v, want %v", native, got, idle)
		}
	}
}

func TestMacroAllowed(t *testing.T) {
	p := &Printer{macros: []string{"M600", "LOAD_FILAMENT"}}
	if !p.macroAllowed("m600") {
		t.Error("case-insensitive allowlisted macro rejected")
	}
	for _, bad := range []string{"M999", "M600\nM112", "M600 ", "M600;G28", ""} {
		if p.macroAllowed(bad) {
			t.Errorf("macro %q should be rejected", bad)
		}
	}
}

func TestModelEnclosed(t *testing.T) {
	for _, m := range []string{"x1", "X1C", "p1s"} {
		if !modelEnclosed(m) {
			t.Errorf("%q should be enclosed", m)
		}
	}
	for _, m := range []string{"p1p", "a1", "a1mini", ""} {
		if modelEnclosed(m) {
			t.Errorf("%q should not be enclosed", m)
		}
	}
}

func TestJFloatTolerant(t *testing.T) {
	m := rep(map[string]string{"a": `1.5`, "b": `"2.5"`, "c": `"oops"`})
	if jfloat(m, "a") != 1.5 || jfloat(m, "b") != 2.5 || jfloat(m, "c") != 0 || jfloat(m, "missing") != 0 {
		t.Errorf("jfloat tolerance failed: %v %v %v %v", jfloat(m, "a"), jfloat(m, "b"), jfloat(m, "c"), jfloat(m, "missing"))
	}
}
