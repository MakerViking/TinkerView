package octoprint

import (
	"testing"

	"github.com/MakerViking/TinkerView/internal/adapter"
)

func TestClassifyState(t *testing.T) {
	cases := []struct {
		text      string
		want      adapter.PState
		connected bool
	}{
		{"Operational", adapter.Idle, true},
		{"Printing", adapter.Printing, true},
		{"Paused", adapter.Paused, true},
		{"Pausing", adapter.Paused, true},
		{"Cancelling", adapter.Cancelled, true},
		{"Error", adapter.Errored, true},
		{"Offline", adapter.Offline, false},
		{"Closed", adapter.Offline, false},
		{"Offline after error", adapter.Offline, false}, // offline wins over error
		{"", adapter.Offline, false},
	}
	for _, c := range cases {
		got, conn := classifyState(c.text)
		if got != c.want || conn != c.connected {
			t.Errorf("classifyState(%q) = (%s,%v), want (%s,%v)", c.text, got, conn, c.want, c.connected)
		}
	}
}

func TestRefineFromFlags(t *testing.T) {
	// closedOrError without the error flag means the port is closed → offline.
	if st, conn := refineFromFlags(octoFlags{ClosedOrError: true}, adapter.Idle, true); st != adapter.Offline || conn {
		t.Errorf("closed port: got (%s,%v)", st, conn)
	}
	if st, _ := refineFromFlags(octoFlags{Printing: true}, adapter.Idle, true); st != adapter.Printing {
		t.Errorf("printing flag: got %s", st)
	}
	// a fresh operational flag must override a stale free-text "printing" fallback.
	if st, _ := refineFromFlags(octoFlags{Operational: true}, adapter.Printing, true); st != adapter.Idle {
		t.Errorf("operational flag should override fallback: got %s", st)
	}
}

func TestHomeAxes(t *testing.T) {
	if got := homeAxes(""); len(got) != 3 {
		t.Errorf("empty axes should home all, got %v", got)
	}
	if got := homeAxes("xy"); len(got) != 2 || got[0] != "x" || got[1] != "y" {
		t.Errorf("homeAxes(xy) = %v", got)
	}
}
