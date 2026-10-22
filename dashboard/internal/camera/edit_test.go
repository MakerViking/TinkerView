package camera

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/MakerViking/TinkerView/internal/adapters/go2rtc"
	"github.com/MakerViking/TinkerView/internal/bus"
	"github.com/MakerViking/TinkerView/internal/devices"
)

// fakeG2 is a minimal go2rtc streams API.
type fakeG2 struct {
	mu      sync.Mutex
	streams map[string]string
	calls   []string // "METHOD name"
	failPut map[string]bool
}

func newFakeG2(t *testing.T) (*fakeG2, *go2rtc.Client) {
	f := &fakeG2{streams: map[string]string{}, failPut: map[string]bool{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		q := r.URL.Query()
		switch r.Method {
		case http.MethodGet:
			out := map[string]any{}
			for n := range f.streams {
				out[n] = map[string]any{}
			}
			_ = json.NewEncoder(w).Encode(out)
		case http.MethodPut, http.MethodPatch:
			name := q.Get("name")
			f.calls = append(f.calls, r.Method+" "+name)
			if f.failPut[name] {
				http.Error(w, "nope", http.StatusBadRequest)
				return
			}
			f.streams[name] = q.Get("src")
		case http.MethodDelete:
			f.calls = append(f.calls, "DELETE "+q.Get("src"))
			delete(f.streams, q.Get("src"))
		}
	}))
	t.Cleanup(srv.Close)
	return f, go2rtc.NewClient(srv.URL)
}

func (f *fakeG2) src(name string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.streams[name]
}

func newTestManager(t *testing.T, fr *FrigateConfig) (*Manager, *devices.Store, *fakeG2) {
	t.Helper()
	store, err := devices.Open(filepath.Join(t.TempDir(), "devices.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	f, client := newFakeG2(t)
	return NewManager(context.Background(), store, client, bus.New(), fr, nil, false), store, f
}

func TestEditCameraCredentialSemantics(t *testing.T) {
	m, store, g := newTestManager(t, nil)
	if err := m.AddCamera("door", "rtsp", "rtsp://10.0.0.5/s", "admin", "pw1"); err != nil {
		t.Fatal(err)
	}
	if got := g.src("door"); got != "rtsp://admin:pw1@10.0.0.5/s" {
		t.Fatalf("add src = %q", got)
	}
	rec := func() devices.CameraRecord { r, _ := store.Camera("door"); return r }

	// Username only (blank password): the password must be KEPT, not wiped.
	if _, err := m.Edit("door", EditRequest{Username: "root"}); err != nil {
		t.Fatal(err)
	}
	if r := rec(); r.Username != "root" || r.Password != "pw1" {
		t.Fatalf("username-only edit: %+v", r)
	}
	// Same username re-typed, blank password: nothing wiped.
	if _, err := m.Edit("door", EditRequest{Username: "root"}); err != nil {
		t.Fatal(err)
	}
	if r := rec(); r.Password != "pw1" {
		t.Fatalf("re-typed username wiped password: %+v", r)
	}
	// Password only (blank username) is applied.
	if _, err := m.Edit("door", EditRequest{Password: "pw2"}); err != nil {
		t.Fatal(err)
	}
	if r := rec(); r.Username != "root" || r.Password != "pw2" {
		t.Fatalf("password-only edit: %+v", r)
	}
	if got := g.src("door"); got != "rtsp://root:pw2@10.0.0.5/s" {
		t.Fatalf("live src = %q", got)
	}
	// Source change with the credentials-stripped URL keeps credentials.
	if _, err := m.Edit("door", EditRequest{NewSrcKind: "rtsp", NewSrcInput: "rtsp://10.0.0.6/s2"}); err != nil {
		t.Fatal(err)
	}
	if got := g.src("door"); got != "rtsp://root:pw2@10.0.0.6/s2" {
		t.Fatalf("live src after source edit = %q", got)
	}
	// Clear removes both.
	if _, err := m.Edit("door", EditRequest{ClearCredentials: true, Username: "ignored"}); err != nil {
		t.Fatal(err)
	}
	if r := rec(); r.Username != "" || r.Password != "" {
		t.Fatalf("clear failed: %+v", r)
	}
	if got := g.src("door"); got != "rtsp://10.0.0.6/s2" {
		t.Fatalf("live src after clear = %q", got)
	}
	// A password with no username anywhere is rejected, store untouched.
	if _, err := m.Edit("door", EditRequest{Password: "x"}); err == nil {
		t.Fatal("password without username must be rejected")
	}
	if r := rec(); r.Password != "" {
		t.Fatalf("rejected edit mutated store: %+v", r)
	}
}

func TestInfoInfersSrcKindAndRedacts(t *testing.T) {
	m, store, _ := newTestManager(t, nil)
	// Legacy records: no SrcKind, credentials embedded in the URL.
	_ = store.UpsertCamera(devices.CameraRecord{Name: "legacy", Src: "ffmpeg:http://u:secret@h/cam#video=h264#hardware"})
	_ = store.UpsertCamera(devices.CameraRecord{Name: "withcreds", Src: "rtsp://h/s", SrcKind: "rtsp", Username: "a", Password: "b"})
	info, ok := m.Info(context.Background(), "legacy")
	if !ok || info.SrcKind != "mjpeg" || info.SrcInput != "http://h/cam" || info.Username != "u" || !info.HasPassword || !info.Editable || !info.Managed {
		t.Fatalf("legacy info: %+v", info)
	}
	b, _ := json.Marshal(info)
	if strings.Contains(string(b), "secret") {
		t.Fatalf("password leaked: %s", b)
	}
	info, _ = m.Info(context.Background(), "withcreds")
	if info.Username != "a" || !info.HasPassword || info.SrcInput != "rtsp://h/s" {
		t.Fatalf("withcreds info: %+v", info)
	}
	if _, ok := m.Info(context.Background(), "ghost"); ok {
		t.Fatal("unknown camera reported")
	}

	// Editing a legacy record without touching credentials keeps the embedded ones.
	m2, store2, g2 := newTestManager(t, nil)
	_ = store2.UpsertCamera(devices.CameraRecord{Name: "legacy", Src: "rtsp://u:secret@h/s"})
	if _, err := m2.Edit("legacy", EditRequest{NewSrcKind: "rtsp", NewSrcInput: "rtsp://h/s2"}); err != nil {
		t.Fatal(err)
	}
	if got := g2.src("legacy"); got != "rtsp://u:secret@h/s2" {
		t.Fatalf("embedded creds lost: %q", got)
	}
	if r, _ := store2.Camera("legacy"); r.SrcKind != "rtsp" {
		t.Fatalf("SrcKind not backfilled: %+v", r)
	}
}

func TestSourceCheckRejectsProcessSchemes(t *testing.T) {
	m, store, _ := newTestManager(t, nil)
	bad := []struct{ kind, in string }{
		{"advanced", "exec:sh -c id"},
		{"raw", "EXEC:id"},
		{"", "echo:foo"},
		{"advanced", "expr:1"},
		{"advanced", "ffmpeg:/etc/passwd"},
		{"advanced", "ffmpeg:http://h/x#input=-f lavfi -i testsrc"},
		{"mjpeg", "/etc/passwd"},
		{"h264", "exec:id"},
		{"advanced", "rtsp://h/s\nexec:id"},
	}
	for _, c := range bad {
		if err := m.AddCamera("x", c.kind, c.in, "", ""); err == nil {
			t.Errorf("AddCamera accepted %q (%s)", c.in, c.kind)
		}
	}
	good := []struct{ kind, in string }{
		{"advanced", "rtsp://h/s"},
		{"advanced", "ffmpeg:http://h/x#video=h264#raw=-vf hflip"},
		{"advanced", "ffmpeg:rtsp://h/x#input=rtsp/udp"},
		{"advanced", "onvif://u:p@h:80"},
		{"mjpeg", "http://h/cam"},
	}
	for i, c := range good {
		if err := m.AddCamera("ok"+string(rune('a'+i)), c.kind, c.in, "", ""); err != nil {
			t.Errorf("AddCamera rejected %q: %v", c.in, err)
		}
	}
	// Edit path too.
	if _, err := m.Edit("oka", EditRequest{NewSrcKind: "advanced", NewSrcInput: "exec:id"}); err == nil {
		t.Fatal("Edit accepted exec:")
	}
	if r, _ := store.Camera("oka"); r.Src != "rtsp://h/s" {
		t.Fatalf("store mutated: %+v", r)
	}
}

func TestRenameCascadesAndRollsBack(t *testing.T) {
	m, store, g := newTestManager(t, nil)
	_ = m.AddCamera("old", "rtsp", "rtsp://h/s", "", "")
	_ = store.UpsertPrinter(devices.PrinterRecord{ID: "p", Kind: "moonraker", Name: "P", BaseURL: "http://x", Camera: "old"})

	// go2rtc rejecting the new stream: nothing changes.
	g.mu.Lock()
	g.failPut["new"] = true
	g.mu.Unlock()
	if _, err := m.Edit("old", EditRequest{NewName: "new"}); err == nil {
		t.Fatal("expected go2rtc failure")
	}
	if _, ok := store.Camera("old"); !ok {
		t.Fatal("camera lost after failed rename")
	}
	if _, ok := store.Camera("new"); ok {
		t.Fatal("new name persisted after failed rename")
	}
	if p, _ := store.Printer("p"); p.Camera != "old" {
		t.Fatalf("printer ref = %q after rollback", p.Camera)
	}
	if g.src("old") == "" {
		t.Fatal("old go2rtc stream was removed")
	}

	g.mu.Lock()
	g.failPut["new"] = false
	g.mu.Unlock()
	if _, err := m.Edit("old", EditRequest{NewName: "new"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Camera("old"); ok {
		t.Fatal("old record remains")
	}
	if r, ok := store.Camera("new"); !ok || r.Src != "rtsp://h/s" {
		t.Fatalf("new record: %+v %v", r, ok)
	}
	if p, _ := store.Printer("p"); p.Camera != "new" {
		t.Fatalf("printer ref not cascaded: %q", p.Camera)
	}
	if g.src("new") == "" || g.src("old") != "" {
		t.Fatalf("go2rtc streams: new=%q old=%q", g.src("new"), g.src("old"))
	}

	// Removing clears the reference.
	if err := m.RemoveCamera("new"); err != nil {
		t.Fatal(err)
	}
	if p, _ := store.Printer("p"); p.Camera != "" {
		t.Fatalf("printer ref after delete: %q", p.Camera)
	}
}

// fakeFrigate serves GET /api/config/raw and records POST /api/config/save.
type fakeFrigate struct {
	mu    sync.Mutex
	cfg   string
	saves []string // save_option per POST
}

func newFakeFrigate(t *testing.T, cfg string) (*fakeFrigate, *FrigateConfig) {
	f := &fakeFrigate{cfg: cfg}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/config/raw":
			_ = json.NewEncoder(w).Encode(f.cfg)
		case r.Method == http.MethodPost && r.URL.Path == "/api/config/save":
			b, _ := io.ReadAll(r.Body)
			f.cfg = string(b)
			f.saves = append(f.saves, r.URL.Query().Get("save_option"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return f, NewFrigateConfig(srv.URL, t.TempDir())
}

func TestEditFrigateConfigStreamSource(t *testing.T) {
	fr, fc := newFakeFrigate(t, sampleConfig)
	m, store, g := newTestManager(t, fc)

	// Name is read-only.
	if _, err := m.Edit("front_door", EditRequest{NewName: "x"}); err == nil {
		t.Fatal("rename of a config stream must be rejected")
	}
	// Unknown stream.
	if _, err := m.Edit("ghost", EditRequest{NewSrcKind: "rtsp", NewSrcInput: "rtsp://h/s"}); err == nil {
		t.Fatal("unknown stream must error")
	}
	// *_hq is never touched.
	if _, err := m.Edit("front_door_hq", EditRequest{NewSrcKind: "rtsp", NewSrcInput: "rtsp://h/s"}); err == nil {
		t.Fatal("_hq stream must be rejected")
	}

	// Info reads it from the Frigate config, credentials stripped.
	info, ok := m.Info(context.Background(), "front_door")
	if !ok || !info.ConfigStream || info.Managed || !info.Editable || info.SrcKind != "rtsp" ||
		info.SrcInput != "rtsp://192.168.1.50:554/Streaming/Channels/101" || !info.HasPassword {
		t.Fatalf("config-stream info: %+v ok=%v", info, ok)
	}

	// Edit the source URL, keep the credentials ({ENV} placeholders).
	async, err := m.Edit("front_door", EditRequest{NewSrcKind: "rtsp", NewSrcInput: "rtsp://192.168.1.51:554/Streaming/Channels/101"})
	if err != nil || async {
		t.Fatalf("edit: async=%v err=%v", async, err)
	}
	fr.mu.Lock()
	cfg, saves := fr.cfg, append([]string(nil), fr.saves...)
	fr.mu.Unlock()
	if !strings.Contains(cfg, "@192.168.1.51:554/Streaming/Channels/101") || !strings.Contains(cfg, "{FRIGATE_RTSP_USER}") {
		t.Fatalf("config not updated / creds lost:\n%s", cfg)
	}
	if !strings.Contains(cfg, "# security cam") {
		t.Fatalf("comments lost:\n%s", cfg)
	}
	for _, s := range saves {
		if s == "restart" {
			t.Fatalf("a restart was triggered: %v", saves)
		}
	}
	// Placeholders can't be expanded by go2rtc, so no live patch -> next restart.
	if got := g.src("front_door"); got != "" {
		t.Fatalf("placeholder source must not be pushed live: %q", got)
	}
	if _, ok := store.Camera("front_door"); ok {
		t.Fatal("plain config stream must not be adopted into the store")
	}

	// A stream without placeholders is patched live (PATCH, no restart).
	_, err = m.Edit("printer1", EditRequest{NewSrcKind: "mjpeg", NewSrcInput: "http://192.168.1.81/webcam/?action=stream"})
	if err != nil {
		t.Fatal(err)
	}
	if got := g.src("printer1"); got != "ffmpeg:http://192.168.1.81/webcam/?action=stream#video=h264" {
		t.Fatalf("live src = %q", got)
	}
	g.mu.Lock()
	calls := strings.Join(g.calls, ",")
	g.mu.Unlock()
	if !strings.Contains(calls, "PATCH printer1") {
		t.Fatalf("expected PATCH, got %s", calls)
	}
	fr.mu.Lock()
	hasNew := strings.Contains(fr.cfg, "192.168.1.81")
	fr.mu.Unlock()
	if !hasNew {
		t.Fatal("config not updated for printer1")
	}

	// New credentials are escaped and injected.
	if _, err := m.Edit("printer1", EditRequest{NewSrcKind: "rtsp", NewSrcInput: "rtsp://192.168.1.81/s", Username: "me", Password: "p@ss"}); err != nil {
		t.Fatal(err)
	}
	if got, want := g.src("printer1"), "rtsp://me:"+url.QueryEscape("p@ss")+"@192.168.1.81/s"; got != want && got != "rtsp://me:p%40ss@192.168.1.81/s" {
		t.Fatalf("live src = %q", got)
	}
}
