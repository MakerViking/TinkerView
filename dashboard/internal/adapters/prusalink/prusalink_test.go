package prusalink

import (
	"encoding/json"
	"net/http"
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

func TestFlexFloatStatusDecode(t *testing.T) {
	// A firmware that stringifies a telemetry value must NOT fail the whole decode
	// (which would drop the printer to offline). Mixed number/string here.
	raw := `{"printer":{"state":"PRINTING","temp_nozzle":"214.9","target_nozzle":215,"temp_bed":59.5},
	         "job":{"id":5,"progress":"42.0","time_remaining":520,"time_printing":60}}`
	var st plStatus
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if float64(st.Printer.TempNozzle) != 214.9 {
		t.Errorf("temp_nozzle (string) = %v", st.Printer.TempNozzle)
	}
	if float64(st.Printer.TargetNozzle) != 215 || float64(st.Printer.TempBed) != 59.5 {
		t.Errorf("numeric temps wrong: %+v", st.Printer)
	}
	if st.Job == nil || float64(st.Job.Progress) != 42.0 {
		t.Errorf("progress (string) wrong: %+v", st.Job)
	}
	// Blank/null tolerated as 0.
	var st2 plStatus
	if err := json.Unmarshal([]byte(`{"printer":{"state":"IDLE","temp_nozzle":""}}`), &st2); err != nil {
		t.Fatalf("blank temp should decode: %v", err)
	}
	if float64(st2.Printer.TempNozzle) != 0 {
		t.Errorf("blank temp should be 0, got %v", st2.Printer.TempNozzle)
	}
}

func TestClassifyLegacy(t *testing.T) {
	if classifyLegacy(legacyFlags{Printing: true}) != adapter.Printing {
		t.Error("printing")
	}
	if classifyLegacy(legacyFlags{Paused: true}) != adapter.Paused {
		t.Error("paused")
	}
	if classifyLegacy(legacyFlags{Error: true}) != adapter.Errored {
		t.Error("error")
	}
	if classifyLegacy(legacyFlags{}) != adapter.Idle {
		t.Error("idle default")
	}
	// printing takes precedence over a stale paused flag.
	if classifyLegacy(legacyFlags{Printing: true, Paused: true}) != adapter.Printing {
		t.Error("printing should win")
	}
}

func TestShouldFallback(t *testing.T) {
	for _, c := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
		if !shouldFallback(c) {
			t.Errorf("status %d should trigger legacy fallback", c)
		}
	}
	for _, c := range []int{0, http.StatusOK, http.StatusInternalServerError, http.StatusBadGateway} {
		if shouldFallback(c) {
			t.Errorf("status %d should NOT trigger fallback (network/server, not protocol)", c)
		}
	}
}

func TestLegacyJobDecode(t *testing.T) {
	// completion is 0..100 OctoPrint-compat and must pass through unscaled; temps
	// tolerate a stringified value.
	raw := `{"job":{"file":{"name":"a.gco","display_name":"Benchy.gcode"}},
	         "progress":{"completion":42.5,"printTime":600,"printTimeLeft":1200}}`
	var jb legacyJob
	if err := json.Unmarshal([]byte(raw), &jb); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if jb.Job == nil || jb.Job.File.DisplayName != "Benchy.gcode" {
		t.Fatalf("filename: %+v", jb.Job)
	}
	if clampPct(float64(jb.Progress.Completion)) != 42.5 {
		t.Errorf("completion should pass through 0..100, got %v", jb.Progress.Completion)
	}
	if clampPct(150) != 100 || clampPct(-5) != 0 {
		t.Error("clampPct bounds")
	}
}

func TestOfflineReason(t *testing.T) {
	if m := offlineReason(http.StatusUnauthorized, nil); !strings.Contains(m, "401") || !strings.Contains(m, "maker") {
		t.Errorf("401 reason should mention auth + maker: %q", m)
	}
	if m := offlineReason(http.StatusOK, errInvalid); !strings.Contains(strings.ToLower(m), "unexpected response") {
		t.Errorf("200+decode-error reason wrong: %q", m)
	}
	if m := offlineReason(0, errInvalid); !strings.Contains(strings.ToLower(m), "unreachable") {
		t.Errorf("transport reason wrong: %q", m)
	}
}

var errInvalid = errInvalidT("boom")

type errInvalidT string

func (e errInvalidT) Error() string { return string(e) }

func TestAuthHeaderMD5Sess(t *testing.T) {
	sess := &Printer{pass: "secret"}
	sess.setChallenge(`Digest realm="r", nonce="n", qop="auth", algorithm=MD5-sess`)
	h := sess.authHeader("GET", "/api/v1/status")
	for _, want := range []string{"algorithm=MD5-sess", "cnonce=", `response="`, "qop=auth"} {
		if !strings.Contains(h, want) {
			t.Errorf("MD5-sess header missing %q\n  got: %s", want, h)
		}
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
