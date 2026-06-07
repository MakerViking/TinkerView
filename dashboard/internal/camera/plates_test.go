package camera

import (
	"reflect"
	"strings"
	"testing"
)

func TestKnownPlatesRoundTrip(t *testing.T) {
	const base = `mqtt:
  host: localhost
lpr:
  enabled: true
cameras: {}
`
	raw, err := setKnownPlatesYAML(base, map[string][]string{
		"Bob":  {"ABC123", "DEF456"},
		"Jane": {"GHI789"},
	})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	// lpr.enabled must be preserved alongside the new known_plates.
	if !strings.Contains(raw, "enabled: true") {
		t.Fatalf("existing lpr config lost:\n%s", raw)
	}
	got, err := parseKnownPlates(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := map[string][]string{"Bob": {"ABC123", "DEF456"}, "Jane": {"GHI789"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip mismatch: got %v want %v", got, want)
	}
}

func TestParseKnownPlatesScalar(t *testing.T) {
	// A single plate may be written as a bare scalar (not a list).
	got, err := parseKnownPlates("lpr:\n  known_plates:\n    Bob: ABC123\n")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !reflect.DeepEqual(got["Bob"], []string{"ABC123"}) {
		t.Fatalf("scalar plate: got %v", got["Bob"])
	}
}

func TestSetKnownPlatesEmptyRemovesKey(t *testing.T) {
	const base = "lpr:\n  enabled: true\n  known_plates:\n    Bob: [ABC123]\n"
	raw, err := setKnownPlatesYAML(base, nil)
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if strings.Contains(raw, "known_plates") {
		t.Fatalf("known_plates should be removed:\n%s", raw)
	}
	if !strings.Contains(raw, "enabled: true") {
		t.Fatalf("lpr block should remain:\n%s", raw)
	}
}

func TestCleanPlates(t *testing.T) {
	out, err := cleanPlates(map[string][]string{
		"  Bob ": {"abc", " abc ", "", "DEF"}, // trims, dedupes (case-insensitive), drops blanks
	})
	if err != nil {
		t.Fatalf("clean: %v", err)
	}
	if !reflect.DeepEqual(out["Bob"], []string{"abc", "DEF"}) {
		t.Fatalf("clean dedupe/trim: got %v", out["Bob"])
	}
	if _, err := cleanPlates(map[string][]string{"Empty": {" ", ""}}); err == nil {
		t.Fatal("expected error for owner with no plates")
	}
	if _, err := cleanPlates(map[string][]string{"  ": {"ABC"}}); err == nil {
		t.Fatal("expected error for blank owner name")
	}
}
