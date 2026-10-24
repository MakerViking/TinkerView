package devices

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/MakerViking/TinkerView/internal/bus"
)

func TestStoreRollsBackWhenSaveFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "devices.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertPrinter(PrinterRecord{ID: "p1", Kind: "moonraker", Name: "P1", BaseURL: "http://x", Camera: "cam"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertCamera(CameraRecord{Name: "cam", Src: "rtsp://h/s"}); err != nil {
		t.Fatal(err)
	}

	// Make every further save fail (the temp file can't be created).
	if err := os.RemoveAll(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}

	if err := s.UpsertPrinter(PrinterRecord{ID: "p2", Kind: "moonraker", Name: "P2", BaseURL: "http://y"}); err == nil {
		t.Fatal("expected save error")
	}
	if err := s.UpsertPrinter(PrinterRecord{ID: "p1", Kind: "moonraker", Name: "Changed", BaseURL: "http://x"}); err == nil {
		t.Fatal("expected save error")
	}
	if err := s.UpsertCamera(CameraRecord{Name: "cam2", Src: "rtsp://h/2"}); err == nil {
		t.Fatal("expected save error")
	}
	if _, err := s.DeleteCamera("cam"); err == nil {
		t.Fatal("expected save error")
	}
	if _, err := s.DeletePrinter("p1"); err == nil {
		t.Fatal("expected save error")
	}
	if _, err := s.RenameCamera("cam", CameraRecord{Name: "renamed", Src: "rtsp://h/s"}); err == nil {
		t.Fatal("expected save error")
	}
	if _, _, err := s.DeleteCameraClearRefs("cam"); err == nil {
		t.Fatal("expected save error")
	}

	ps, cs := s.Printers(), s.Cameras()
	if len(ps) != 1 || ps[0].ID != "p1" || ps[0].Name != "P1" || ps[0].Camera != "cam" {
		t.Fatalf("printers not rolled back: %+v", ps)
	}
	if len(cs) != 1 || cs[0].Name != "cam" {
		t.Fatalf("cameras not rolled back: %+v", cs)
	}
}

func TestRenameCameraCascadesAndDeleteClearsRefs(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "devices.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	_ = s.UpsertCamera(CameraRecord{Name: "mk4cam", Src: "rtsp://h/1"})
	_ = s.UpsertCamera(CameraRecord{Name: "other", Src: "rtsp://h/2"})
	_ = s.UpsertPrinter(PrinterRecord{ID: "mk4", Kind: "moonraker", Name: "MK4", BaseURL: "http://x", Camera: "mk4cam"})
	_ = s.UpsertPrinter(PrinterRecord{ID: "q", Kind: "moonraker", Name: "Q", BaseURL: "http://y", Camera: "other"})

	var hooked []string
	s.SetPrinterRefHook(func(ids []string) { hooked = append(hooked, ids...) })

	if _, err := s.RenameCamera("mk4cam", CameraRecord{Name: "other", Src: "x"}); err == nil {
		t.Fatal("rename onto an existing name must fail")
	}
	if _, err := s.RenameCamera("nope", CameraRecord{Name: "new", Src: "x"}); err == nil {
		t.Fatal("rename of an unknown camera must fail")
	}
	ids, err := s.RenameCamera("mk4cam", CameraRecord{Name: "garage", Src: "rtsp://h/1"})
	if err != nil || len(ids) != 1 || ids[0] != "mk4" {
		t.Fatalf("rename: ids=%v err=%v", ids, err)
	}
	if _, ok := s.Camera("mk4cam"); ok {
		t.Fatal("old name still present")
	}
	if p, _ := s.Printer("mk4"); p.Camera != "garage" {
		t.Fatalf("printer ref not cascaded: %q", p.Camera)
	}
	if p, _ := s.Printer("q"); p.Camera != "other" {
		t.Fatalf("unrelated printer touched: %q", p.Camera)
	}
	if len(hooked) != 1 || hooked[0] != "mk4" {
		t.Fatalf("hook = %v", hooked)
	}

	found, ids, err := s.DeleteCameraClearRefs("garage")
	if err != nil || !found || len(ids) != 1 {
		t.Fatalf("delete: found=%v ids=%v err=%v", found, ids, err)
	}
	if p, _ := s.Printer("mk4"); p.Camera != "" {
		t.Fatalf("printer ref not cleared: %q", p.Camera)
	}
	// Persisted.
	s2, _ := Open(s.path)
	if p, _ := s2.Printer("mk4"); p.Camera != "" {
		t.Fatal("cleared ref not persisted")
	}
}

func TestPrinterRecordJSONRedactsSecrets(t *testing.T) {
	r := PrinterRecord{ID: "b", Kind: "bambu", Name: "B", IP: "1.2.3.4", Serial: "S", AccessCode: "sekret-code", APIKey: "sekret-key"}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	if strings.Contains(out, "sekret") || strings.Contains(out, "accessCode\"") || strings.Contains(out, "apiKey\"") {
		t.Fatalf("secret leaked: %s", out)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m["hasApiKey"] != true || m["hasAccessCode"] != true || m["managed"] != true {
		t.Fatalf("flags missing: %s", out)
	}
	if _, ok := m["macros"].([]any); !ok {
		t.Fatalf("macros must be a JSON array: %s", out)
	}
	// Request bodies must still be accepted with their secrets.
	var in PrinterRecord
	if err := json.Unmarshal([]byte(`{"id":"x","kind":"octoprint","apiKey":"k","accessCode":"c"}`), &in); err != nil {
		t.Fatal(err)
	}
	if in.APIKey != "k" || in.AccessCode != "c" {
		t.Fatalf("decode dropped secrets: %+v", in)
	}
}

func TestInferSrcKindAndInput(t *testing.T) {
	cases := []struct{ src, kind, input string }{
		{"rtsp://u:p@h/stream", "rtsp", "rtsp://h/stream"},
		{"ffmpeg:http://h/cam#video=h264#hardware", "mjpeg", "http://h/cam"},
		{"ffmpeg:http://h/cam#video=h264", "h264", "http://h/cam"},
		{"http://h/mjpeg", "mjpeg_raw", "http://h/mjpeg"},
		{"exec:ffmpeg -i x", "advanced", "exec:ffmpeg -i x"},
		{"ffmpeg:http://h/cam#video=h264#raw=-vf hflip", "advanced", "ffmpeg:http://h/cam#video=h264#raw=-vf hflip"},
		{"rtsp://{FRIGATE_RTSP_USER}:{FRIGATE_RTSP_PASSWORD}@10.0.0.5/s", "rtsp", "rtsp://10.0.0.5/s"},
	}
	for _, c := range cases {
		k := InferSrcKind(c.src)
		if k != c.kind {
			t.Errorf("InferSrcKind(%q) = %q, want %q", c.src, k, c.kind)
		}
		if got := SrcInputOf(k, c.src); got != c.input {
			t.Errorf("SrcInputOf(%q,%q) = %q, want %q", k, c.src, got, c.input)
		}
	}
	if got := WithUserinfo("rtsp://h/s", "a:b"); got != "rtsp://a:b@h/s" {
		t.Errorf("WithUserinfo = %q", got)
	}
	if got := WithUserinfo("ffmpeg:rtsp://x:y@h/s#video=h264", ""); got != "ffmpeg:rtsp://h/s#video=h264" {
		t.Errorf("WithUserinfo strip = %q", got)
	}
}

// ----- EditPrinter -----

func newTestManager(t *testing.T) (*Manager, *Store) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	t.Cleanup(func() { cancel(); wg.Wait() })
	store, err := Open(filepath.Join(t.TempDir(), "devices.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	run := func(_ string, fn func(context.Context)) {
		wg.Add(1)
		go func() { defer wg.Done(); fn(ctx) }()
	}
	return NewManager(ctx, bus.New(), store, run), store
}

func TestEditPrinterBlankKeepsAndClears(t *testing.T) {
	m, store := newTestManager(t)
	if err := m.AddPrinter(PrinterRecord{
		ID: "op", Kind: "octoprint", Name: "Old", BaseURL: "http://127.0.0.1:1", APIKey: "KEY",
		Camera: "cam", UIURL: "http://ui", Model: "m",
	}); err != nil {
		t.Fatal(err)
	}

	// Blank secret / base URL keep; blank uiUrl and camera clear; name changes.
	if err := m.EditPrinter("op", PrinterRecord{Kind: "octoprint", Name: "New"}); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Printer("op")
	if got.Name != "New" || got.APIKey != "KEY" || got.BaseURL != "http://127.0.0.1:1" || got.Model != "m" {
		t.Fatalf("blank fields not kept: %+v", got)
	}
	if got.UIURL != "" || got.Camera != "" {
		t.Fatalf("blank uiUrl/camera must clear: %+v", got)
	}

	// A new secret replaces.
	if err := m.EditPrinter("op", PrinterRecord{Kind: "octoprint", Name: "New", APIKey: "KEY2"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Printer("op"); got.APIKey != "KEY2" {
		t.Fatalf("new key not saved: %+v", got)
	}

	// Kind is immutable.
	if err := m.EditPrinter("op", PrinterRecord{Kind: "moonraker", Name: "New", BaseURL: "http://127.0.0.1:1"}); err == nil {
		t.Fatal("kind change must be rejected")
	}
	if got, _ := store.Printer("op"); got.Kind != "octoprint" {
		t.Fatalf("kind changed despite rejection: %+v", got)
	}
	// Unknown printer.
	if err := m.EditPrinter("nope", PrinterRecord{Kind: "octoprint", Name: "x"}); err == nil {
		t.Fatal("unknown printer must error")
	}
	// A still-live adapter after edits.
	m.mu.Lock()
	_, running := m.printers["op"]
	m.mu.Unlock()
	if !running {
		t.Fatal("printer not running after edit")
	}
}
