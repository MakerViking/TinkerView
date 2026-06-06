package prusalink

import (
	"strings"
	"testing"

	"github.com/MakerViking/TinkerView/internal/adapter"
)

func TestClassifyPState(t *testing.T) {
	cases := map[string]adapter.PState{
		"IDLE":      adapter.Idle,
		"READY":     adapter.Idle,
		"BUSY":      adapter.Idle,
		"PRINTING":  adapter.Printing,
		"PAUSED":    adapter.Paused,
		"FINISHED":  adapter.Complete,
		"STOPPED":   adapter.Cancelled,
		"ERROR":     adapter.Errored,
		"ATTENTION": adapter.Errored,
		"weird":     adapter.Idle,
	}
	for in, want := range cases {
		if got := classifyPState(in); got != want {
			t.Errorf("classifyPState(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestToStateFlags(t *testing.T) {
	p := &Printer{online: true, pstate: adapter.Printing, progressPct: 42, nozzle: adapter.Heater{Actual: 215, Target: 215}}
	s := p.toState()
	if s.State != adapter.Printing || !s.Flags.CanPause || !s.Flags.CanCancel || s.Flags.CanResume {
		t.Errorf("printing flags wrong: %+v", s.Flags)
	}
	if s.ProgressPct != 42 {
		t.Errorf("progress should pass through 0..100, got %v", s.ProgressPct)
	}
	if s.Caps != nil {
		t.Errorf("PrusaLink must advertise no caps, got %+v", s.Caps)
	}

	pp := &Printer{online: true, pstate: adapter.Paused}
	if f := pp.toState().Flags; !f.CanResume || !f.CanCancel || f.CanPause {
		t.Errorf("paused flags wrong: %+v", f)
	}

	off := &Printer{online: false, offlineMsg: "PrusaLink unreachable"}
	if s := off.toState(); s.State != adapter.Offline || s.Message == "" {
		t.Errorf("offline state wrong: %+v", s)
	}
}

func TestParseChallenge(t *testing.T) {
	www := `Digest realm="Printer API", nonce="abc123==", qop="auth", algorithm=MD5, opaque="op,aque"`
	m := parseChallenge(www)
	if m["realm"] != "Printer API" {
		t.Errorf("realm = %q", m["realm"])
	}
	if m["nonce"] != "abc123==" {
		t.Errorf("nonce = %q", m["nonce"])
	}
	if m["qop"] != "auth" {
		t.Errorf("qop = %q", m["qop"])
	}
	if m["algorithm"] != "MD5" {
		t.Errorf("algorithm = %q", m["algorithm"])
	}
	if m["opaque"] != "op,aque" { // comma inside quotes must not split
		t.Errorf("opaque = %q (quoted comma not handled)", m["opaque"])
	}
	if parseChallenge("Basic realm=x") != nil {
		t.Error("non-Digest header should return nil")
	}
}

func TestMd5hex(t *testing.T) {
	// RFC 1321 test vector.
	if got := md5hex("abc"); got != "900150983cd24fb0d6963f7d28e17f72" {
		t.Errorf("md5hex(abc) = %s", got)
	}
}

func TestAuthHeaderShape(t *testing.T) {
	p := &Printer{pass: "secret"}
	if h := p.authHeader("GET", "/api/v1/status"); h != "" {
		t.Errorf("no challenge yet should yield empty header, got %q", h)
	}
	p.setChallenge(`Digest realm="r", nonce="n", qop="auth", algorithm=MD5`)
	h := p.authHeader("GET", "/api/v1/status")
	for _, want := range []string{
		`username="maker"`, `realm="r"`, `nonce="n"`, `uri="/api/v1/status"`,
		"qop=auth", "nc=00000001", "algorithm=MD5", "cnonce=", `response="`,
	} {
		if !strings.Contains(h, want) {
			t.Errorf("auth header missing %q\n  got: %s", want, h)
		}
	}
	// nc must advance on the next request.
	if h2 := p.authHeader("GET", "/api/v1/status"); !strings.Contains(h2, "nc=00000002") {
		t.Errorf("nc did not advance: %s", h2)
	}
}
