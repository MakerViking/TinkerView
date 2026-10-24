package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MakerViking/TinkerView/internal/adapters/go2rtc"
	"github.com/MakerViking/TinkerView/internal/bus"
	"github.com/MakerViking/TinkerView/internal/camera"
	"github.com/MakerViking/TinkerView/internal/devices"
)

func newDevAPI(t *testing.T) (*API, *devices.Store) {
	t.Helper()
	g2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte("{}"))
		}
	}))
	t.Cleanup(g2.Close)
	store, err := devices.Open(filepath.Join(t.TempDir(), "devices.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	b := bus.New()
	pm := devices.NewManager(ctx, b, store, func(_ string, fn func(context.Context)) { go fn(ctx) })
	cm := camera.NewManager(ctx, store, go2rtc.NewClient(g2.URL), b, nil, nil, false)
	a := &API{cmd: nil}
	a.SetDevices(pm, cm)
	return a, store
}

func serve(a *API, method, path, body string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	a.Register(mux)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestPrinterGetRedactsSecrets(t *testing.T) {
	a, store := newDevAPI(t)
	_ = store.UpsertPrinter(devices.PrinterRecord{
		ID: "b1", Kind: "bambu", Name: "Bambu", IP: "10.0.0.9", Serial: "SER", AccessCode: "ACCESS-SECRET",
		APIKey: "API-SECRET", Model: "x1c", Camera: "cam", UIURL: "http://ui",
	})
	rec := serve(a, "GET", "/api/printers/b1", "")
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if strings.Contains(body, "SECRET") || strings.Contains(body, `"apiKey"`) || strings.Contains(body, `"accessCode"`) {
		t.Fatalf("secret leaked: %s", body)
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"id", "kind", "name", "baseUrl", "ip", "serial", "model", "camera", "uiUrl", "macros", "hasApiKey", "hasAccessCode", "managed"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing field %q in %s", k, body)
		}
	}
	if m["hasApiKey"] != true || m["hasAccessCode"] != true || m["managed"] != true || m["serial"] != "SER" {
		t.Fatalf("unexpected values: %s", body)
	}
	if rec := serve(a, "GET", "/api/printers/nope", ""); rec.Code != 404 || !strings.Contains(rec.Body.String(), `"error"`) {
		t.Fatalf("unknown printer: %d %s", rec.Code, rec.Body)
	}
}

func TestCameraGetAndEditErrorShape(t *testing.T) {
	a, store := newDevAPI(t)
	_ = store.UpsertCamera(devices.CameraRecord{Name: "door", Src: "rtsp://h/s", Username: "u", Password: "TOPSECRET"})

	rec := serve(a, "GET", "/api/cameras/door", "")
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "TOPSECRET") {
		t.Fatalf("get: %d %s", rec.Code, rec.Body)
	}
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	for _, k := range []string{"name", "srcKind", "srcInput", "username", "hasPassword", "managed", "promoted", "configStream", "editable", "rotate", "flipH", "flipV"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing field %q in %s", k, rec.Body)
		}
	}
	if m["srcKind"] != "rtsp" || m["hasPassword"] != true || m["editable"] != true {
		t.Fatalf("unexpected: %s", rec.Body)
	}
	if rec := serve(a, "GET", "/api/cameras/ghost", ""); rec.Code != 404 {
		t.Fatalf("unknown camera: %d", rec.Code)
	}

	// A rejected edit is a 4xx with a JSON error body.
	rec = serve(a, "PUT", "/api/cameras/door", `{"newSrcKind":"advanced","newSrcInput":"exec:id"}`)
	var e map[string]string
	if rec.Code != 400 || json.Unmarshal(rec.Body.Bytes(), &e) != nil || e["error"] == "" {
		t.Fatalf("edit error shape: %d %s", rec.Code, rec.Body)
	}
	// Clearing credentials works and is reflected.
	rec = serve(a, "PUT", "/api/cameras/door", `{"clearCredentials":true}`)
	if rec.Code != 200 {
		t.Fatalf("clear: %d %s", rec.Code, rec.Body)
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	if m["username"] != "" || m["hasPassword"] != false {
		t.Fatalf("credentials not cleared: %s", rec.Body)
	}
}
