package rules

import (
	"path/filepath"
	"testing"
	"time"
)

func at(hour, min int) time.Time {
	return time.Date(2026, 6, 8, hour, min, 0, 0, time.UTC)
}

func TestEvaluateDefaults(t *testing.T) {
	s := &Store{rules: DefaultRules()}

	// Recognized face → silent regardless of object/time.
	d := s.Evaluate(Input{Camera: "outside", Label: "person", SubLabel: "Bob", Now: at(23, 0)})
	if !d.Notify || d.Priority != 1 {
		t.Fatalf("known person: want silent push (prio 1), got %+v", d)
	}

	// Unknown person at night → loud.
	d = s.Evaluate(Input{Camera: "outside", Label: "person", SubLabel: "", Now: at(23, 0)})
	if !d.Notify || d.Priority != 5 {
		t.Fatalf("unknown night person: want prio 5, got %+v", d)
	}

	// Unknown person during the day → normal person rule (prio 4).
	d = s.Evaluate(Input{Camera: "outside", Label: "person", SubLabel: "", Now: at(13, 0)})
	if !d.Notify || d.Priority != 4 {
		t.Fatalf("daytime person: want prio 4, got %+v", d)
	}

	// Car → prio 3.
	d = s.Evaluate(Input{Camera: "drive", Label: "car", Now: at(13, 0)})
	if !d.Notify || d.Priority != 3 {
		t.Fatalf("car: want prio 3, got %+v", d)
	}

	// Unmatched object (dog) → default push prio 3.
	d = s.Evaluate(Input{Camera: "yard", Label: "dog", Now: at(13, 0)})
	if !d.Notify || d.Priority != 3 || d.Rule != "" {
		t.Fatalf("dog fallthrough: want default prio 3, got %+v", d)
	}
}

func TestZoneAndCameraSelectors(t *testing.T) {
	s := &Store{rules: []Rule{
		{Name: "yard only", Camera: "outside", Zone: "front_yard", Action: ActionPush, Priority: 5},
		{Name: "everything else", Action: ActionIgnore},
	}}
	// Matches camera + zone.
	if d := s.Evaluate(Input{Camera: "outside", Zones: []string{"front_yard"}, Now: at(12, 0)}); d.Priority != 5 {
		t.Fatalf("zone match: got %+v", d)
	}
	// Right camera, wrong zone → falls to ignore.
	if d := s.Evaluate(Input{Camera: "outside", Zones: []string{"driveway"}, Now: at(12, 0)}); d.Notify {
		t.Fatalf("zone miss should ignore: got %+v", d)
	}
	// Wrong camera → ignore.
	if d := s.Evaluate(Input{Camera: "garage", Zones: []string{"front_yard"}, Now: at(12, 0)}); d.Notify {
		t.Fatalf("camera miss should ignore: got %+v", d)
	}
}

func TestInWindow(t *testing.T) {
	cases := []struct {
		from, to string
		t        time.Time
		want     bool
	}{
		{"", "", at(3, 0), true},                  // all-day
		{"21:00", "06:00", at(23, 30), true},      // overnight, late
		{"21:00", "06:00", at(5, 0), true},        // overnight, early
		{"21:00", "06:00", at(12, 0), false},      // overnight, midday excluded
		{"09:00", "17:00", at(12, 0), true},       // daytime inside
		{"09:00", "17:00", at(8, 0), false},       // before
		{"09:00", "17:00", at(17, 0), false},      // end exclusive
	}
	for _, c := range cases {
		if got := inWindow(c.from, c.to, c.t); got != c.want {
			t.Errorf("inWindow(%q,%q,%v)=%v want %v", c.from, c.to, c.t, got, c.want)
		}
	}
}

func TestSpecificNameMatch(t *testing.T) {
	s := &Store{rules: []Rule{
		{Name: "Alice silent", Match: "alice", Action: ActionSilent},
		{Name: "known loud", Match: MatchKnown, Action: ActionPush, Priority: 4},
	}}
	if d := s.Evaluate(Input{SubLabel: "Alice", Now: at(12, 0)}); d.Priority != 1 {
		t.Fatalf("named match (case-insensitive): got %+v", d)
	}
	if d := s.Evaluate(Input{SubLabel: "Bob", Now: at(12, 0)}); d.Priority != 4 {
		t.Fatalf("other known: got %+v", d)
	}
}

func TestSetValidation(t *testing.T) {
	s := &Store{path: filepath.Join(t.TempDir(), "r.json")}
	if err := s.Set([]Rule{{Action: "bogus"}}); err == nil {
		t.Fatal("expected error for bad action")
	}
	if err := s.Set([]Rule{{Action: ActionPush, Priority: 9}}); err == nil {
		t.Fatal("expected error for out-of-range priority")
	}
	if err := s.Set([]Rule{{Action: ActionPush, From: "25:00", To: "06:00"}}); err == nil {
		t.Fatal("expected error for bad time")
	}
	// Empty action defaults to push.
	if err := s.Set([]Rule{{Name: "ok"}}); err != nil {
		t.Fatalf("valid rule rejected: %v", err)
	}
	if got := s.Get(); len(got) != 1 || got[0].Action != ActionPush {
		t.Fatalf("empty action should default to push, got %+v", got)
	}
}

func TestOpenPersistRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notify-rules.json")
	// Missing file → seeds defaults and writes them.
	s, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if len(s.Get()) != len(DefaultRules()) {
		t.Fatalf("expected seeded defaults")
	}
	// Reopen reads what was written.
	if err := s.Set([]Rule{{Name: "only", Action: ActionIgnore}}); err != nil {
		t.Fatalf("set: %v", err)
	}
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := s2.Get(); len(got) != 1 || got[0].Name != "only" {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}
