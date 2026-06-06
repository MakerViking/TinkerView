package printerutil

import "testing"

func TestClampFeed(t *testing.T) {
	if got := ClampFeed(0, "z"); got != 600 {
		t.Errorf("default z feed = %v, want 600", got)
	}
	if got := ClampFeed(99999, "x"); got != 6000 {
		t.Errorf("xy feed cap = %v, want 6000", got)
	}
	if got := ClampFeed(10, "x"); got != 60 {
		t.Errorf("feed floor = %v, want 60", got)
	}
	if got := ClampFeed(0, "e"); got != 300 {
		t.Errorf("default e feed = %v, want 300", got)
	}
}

func TestClampAbs(t *testing.T) {
	if got := ClampAbs(5, 0.5); got != 0.5 {
		t.Errorf("ClampAbs(5,0.5) = %v, want 0.5", got)
	}
	if got := ClampAbs(-200, 100); got != -100 {
		t.Errorf("ClampAbs(-200,100) = %v, want -100", got)
	}
}

func TestHomeGcode(t *testing.T) {
	if got := HomeGcode(""); got != "G28" {
		t.Errorf("HomeGcode(\"\") = %q, want G28", got)
	}
	if got := HomeGcode("xz"); got != "G28 X Z" {
		t.Errorf("HomeGcode(xz) = %q, want \"G28 X Z\"", got)
	}
}

func TestMacroAllowed(t *testing.T) {
	allowed := []string{"M600", "LOAD_FILAMENT"}
	if !MacroAllowed("m600", allowed) {
		t.Error("case-insensitive allowlisted macro rejected")
	}
	for _, bad := range []string{"M999", "M600\nM112", "M600 ", "M600;G28", ""} {
		if MacroAllowed(bad, allowed) {
			t.Errorf("macro %q should be rejected", bad)
		}
	}
}
