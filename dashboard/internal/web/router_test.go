package web

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/MakerViking/TinkerView/internal/bus"
	"github.com/MakerViking/TinkerView/internal/config"
	"github.com/MakerViking/TinkerView/internal/hub"
)

// upstream records what the proxies forwarded.
type upstream struct {
	*httptest.Server
	hits atomic.Int32
	last atomic.Pointer[http.Request]
}

func newUpstream(t *testing.T) *upstream {
	u := &upstream{}
	u.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.hits.Add(1)
		u.last.Store(r)
		_, _ = w.Write([]byte("up"))
	}))
	t.Cleanup(u.Close)
	return u
}

func newRouter(t *testing.T, token string, g, f *upstream) http.Handler {
	t.Helper()
	cfg := &config.Config{Go2RTCURL: g.URL}
	cfg.Frigate.BaseURL = f.URL
	cfg.Auth.Token = token
	h := hub.New(bus.New(), nil, token)
	api := NewAPI("test", h, nil, nil, nil, nil)
	r, err := Router(cfg, h, api, nil)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func do(h http.Handler, method, target string, mod func(*http.Request)) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	if mod != nil {
		mod(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestProxyAuthGating(t *testing.T) {
	g, f := newUpstream(t), newUpstream(t)
	open := newRouter(t, "", g, f)
	if c := do(open, "GET", "/go2rtc/api/streams", nil).Code; c != 200 {
		t.Fatalf("no token configured: want 200, got %d", c)
	}

	h := newRouter(t, "s3cret", g, f)
	for _, p := range []string{"/go2rtc/api/streams", "/frigate/api/events"} {
		if c := do(h, "GET", p, nil).Code; c != 401 {
			t.Errorf("%s without token: want 401, got %d", p, c)
		}
		if c := do(h, "GET", p+"?token=wrong", nil).Code; c != 401 {
			t.Errorf("%s wrong token: want 401, got %d", p, c)
		}
	}
	before := g.hits.Load() + f.hits.Load()
	// Header, query and cookie all authenticate and the token is stripped upstream.
	rec := do(h, "GET", "/go2rtc/api/frame.jpeg?src=a&token=s3cret", func(r *http.Request) {
		r.Header.Set("Cookie", "other=1")
	})
	if rec.Code != 200 {
		t.Fatalf("query token: got %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Set-Cookie"), authCookie+"=") {
		t.Error("expected auth cookie to be set")
	}
	up := g.last.Load()
	if up.URL.Query().Has("token") || up.URL.Query().Get("src") != "a" {
		t.Errorf("upstream query not stripped/preserved: %q", up.URL.RawQuery)
	}
	if up.URL.Path != "/api/frame.jpeg" {
		t.Errorf("upstream path = %q", up.URL.Path)
	}

	rec = do(h, "GET", "/frigate/api/events", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer s3cret")
	})
	if rec.Code != 200 {
		t.Fatalf("bearer: got %d", rec.Code)
	}
	if f.last.Load().Header.Get("Authorization") != "" {
		t.Error("Authorization forwarded upstream")
	}

	ck := cookieValue("s3cret")
	rec = do(h, "GET", "/frigate/api/events", func(r *http.Request) {
		r.Header.Set("Cookie", authCookie+"="+ck+"; frigate=1")
	})
	if rec.Code != 200 {
		t.Fatalf("cookie: got %d", rec.Code)
	}
	if got := f.last.Load().Header.Get("Cookie"); got != "frigate=1" {
		t.Errorf("upstream cookie = %q, want only frigate=1", got)
	}
	if g.hits.Load()+f.hits.Load() != before+3 {
		t.Error("unexpected upstream hit count")
	}
	// The cookie is NOT accepted for /api/.
	if c := do(h, "GET", "/api/version", func(r *http.Request) {
		r.Header.Set("Cookie", authCookie+"="+ck)
	}).Code; c != 401 {
		t.Errorf("/api with cookie only: want 401, got %d", c)
	}
	if c := do(h, "GET", "/api/version", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer s3cret")
	}).Code; c != 200 {
		t.Errorf("/api with bearer: want 200, got %d", c)
	}
	// Static assets stay open.
	if c := do(h, "GET", "/", nil).Code; c != 200 {
		t.Errorf("SPA with token configured: got %d", c)
	}
}

func TestGo2RTCReadOnly(t *testing.T) {
	g, f := newUpstream(t), newUpstream(t)
	h := newRouter(t, "", g, f)
	for _, m := range []string{"PUT", "POST", "DELETE", "PATCH"} {
		if c := do(h, m, "/go2rtc/api/config", nil).Code; c != 405 {
			t.Errorf("%s /go2rtc/api/config: want 405, got %d", m, c)
		}
	}
	if g.hits.Load() != 0 {
		t.Error("mutating request reached go2rtc")
	}
	for _, p := range []string{"/go2rtc/api/streams", "/go2rtc/api/stream.mjpeg?src=a", "/go2rtc/api/frame.jpeg?src=a"} {
		if c := do(h, "GET", p, nil).Code; c != 200 {
			t.Errorf("GET %s: got %d", p, c)
		}
	}
	// Ad-hoc sources (scheme in src) must not reach go2rtc, even via GET.
	before := g.hits.Load()
	for _, p := range []string{
		"/go2rtc/api/stream.mjpeg?src=rtsp://10.0.0.9/x",
		"/go2rtc/api/frame.jpeg?src=exec:id",
		"/go2rtc/api/ws?src=a&src=ffmpeg:foo%23video=h264",
	} {
		if c := do(h, "GET", p, nil).Code; c != 403 {
			t.Errorf("GET %s: want 403, got %d", p, c)
		}
	}
	if g.hits.Load() != before {
		t.Error("ad-hoc src request reached go2rtc")
	}
	// Frigate writes (face enrollment) still go through.
	if c := do(h, "POST", "/frigate/api/faces/bob/create", nil).Code; c != 200 {
		t.Errorf("frigate POST: got %d", c)
	}
}

func TestCSRFOriginCheck(t *testing.T) {
	g, f := newUpstream(t), newUpstream(t)
	h := newRouter(t, "", g, f)
	post := func(target, origin string) int {
		return do(h, "POST", target, func(r *http.Request) {
			r.Host = "tower.local:8088"
			if origin != "" {
				r.Header.Set("Origin", origin)
			}
		}).Code
	}
	if c := post("/api/notify/test", "http://evil.example"); c != 403 {
		t.Errorf("cross-origin: want 403, got %d", c)
	}
	if c := post("/api/notify/test", "null"); c != 403 {
		t.Errorf("null origin: want 403, got %d", c)
	}
	if c := post("/api/notify/test", "http://tower.local:8088"); c == 403 {
		t.Error("same-origin rejected")
	}
	if c := post("/api/notify/test", ""); c == 403 {
		t.Error("absent Origin rejected")
	}
	if c := post("/frigate/api/faces/x/create", "http://evil.example"); c != 403 {
		t.Errorf("frigate cross-origin: want 403, got %d", c)
	}
	if f.hits.Load() != 0 {
		t.Error("cross-origin request reached Frigate")
	}
	if c := post("/frigate/api/faces/x/create", "http://tower.local:8088"); c != 200 {
		t.Errorf("frigate same-origin: got %d", c)
	}
	// GET with a foreign Origin (e.g. cross-site <img>) is not a CSRF concern.
	if c := do(h, "GET", "/api/version", func(r *http.Request) {
		r.Host = "tower.local:8088"
		r.Header.Set("Origin", "http://evil.example")
	}).Code; c != 200 {
		t.Errorf("GET foreign origin: got %d", c)
	}
}

func TestAPIBodyLimit(t *testing.T) {
	var read int64
	h := withRequestGuards(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, err := io.Copy(io.Discard, r.Body)
		read = n
		if err != nil {
			http.Error(w, "too big", http.StatusRequestEntityTooLarge)
		}
	}))
	req := httptest.NewRequest("POST", "/api/cameras", bytes.NewReader(make([]byte, 2<<20)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge || read > maxAPIBody {
		t.Errorf("body not capped: code=%d read=%d", rec.Code, read)
	}
}

func TestSPACacheHeadersAndFallback(t *testing.T) {
	g, f := newUpstream(t), newUpstream(t)
	h := newRouter(t, "", g, f)

	idx := do(h, "GET", "/", nil)
	if idx.Code != 200 || idx.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("index: code=%d cc=%q", idx.Code, idx.Header().Get("Cache-Control"))
	}
	for _, hd := range []string{"X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy"} {
		if idx.Header().Get(hd) == "" {
			t.Errorf("missing %s on SPA", hd)
		}
	}
	// /assets/ is a directory: must serve the SPA, not a listing.
	for _, p := range []string{"/assets", "/assets/", "/some/client/route"} {
		rec := do(h, "GET", p, nil)
		if rec.Code != 200 || !strings.Contains(rec.Header().Get("Content-Type"), "text/html") ||
			strings.Contains(rec.Body.String(), "<pre>") {
			t.Errorf("%s: code=%d ct=%q", p, rec.Code, rec.Header().Get("Content-Type"))
		}
		if rec.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("%s fallback cache = %q", p, rec.Header().Get("Cache-Control"))
		}
	}

	// A hashed asset (whatever the current build produced) is immutable.
	entries, _ := dist.ReadDir("dist/assets")
	var asset string
	for _, e := range entries {
		if !e.IsDir() {
			asset = "/assets/" + e.Name()
			break
		}
	}
	if asset == "" {
		t.Skip("no built assets embedded")
	}
	rec := do(h, "GET", asset, nil)
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") || !strings.Contains(got, "max-age=31536000") {
		t.Errorf("%s cache = %q", asset, got)
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag")
	}
	if c := do(h, "GET", asset, func(r *http.Request) { r.Header.Set("If-None-Match", etag) }).Code; c != 304 {
		t.Errorf("conditional GET: want 304, got %d", c)
	}
	// Non-hashed files revalidate.
	if cc := do(h, "GET", "/video-rtc.js", nil).Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("video-rtc.js cache = %q", cc)
	}
}

func TestSPAGzipNegotiation(t *testing.T) {
	g, f := newUpstream(t), newUpstream(t)
	h := newRouter(t, "", g, f)
	// video-rtc.js is a large embedded text file.
	plain := do(h, "GET", "/video-rtc.js", nil)
	if plain.Header().Get("Content-Encoding") != "" {
		t.Error("compressed without Accept-Encoding")
	}
	if !strings.Contains(plain.Header().Get("Vary"), "Accept-Encoding") {
		t.Error("missing Vary")
	}
	gz := do(h, "GET", "/video-rtc.js", func(r *http.Request) { r.Header.Set("Accept-Encoding", "gzip, deflate") })
	if gz.Header().Get("Content-Encoding") != "gzip" {
		t.Fatal("expected gzip")
	}
	zr, err := gzip.NewReader(gz.Body)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(zr)
	if !bytes.Equal(got, plain.Body.Bytes()) {
		t.Error("gzip body does not round-trip")
	}
	if gz.Header().Get("ETag") == plain.Header().Get("ETag") {
		t.Error("gzip and identity must have distinct ETags")
	}
	if do(h, "GET", "/video-rtc.js", func(r *http.Request) { r.Header.Set("Accept-Encoding", "gzip;q=0") }).Header().Get("Content-Encoding") != "" {
		t.Error("gzip;q=0 must not compress")
	}
}

func TestGzipMiddleware(t *testing.T) {
	json := strings.Repeat(`{"a":"b"}`, 200)
	h := withGzip(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "png") {
			w.Header().Set("Content-Type", "image/png")
		} else {
			w.Header().Set("Content-Type", "application/json")
		}
		_, _ = w.Write([]byte(json))
	}))
	req := func(path, ae string) *httptest.ResponseRecorder {
		return do(h, "GET", path, func(r *http.Request) {
			if ae != "" {
				r.Header.Set("Accept-Encoding", ae)
			}
		})
	}
	rec := req("/api/state", "gzip")
	if rec.Header().Get("Content-Encoding") != "gzip" || rec.Header().Get("Vary") != "Accept-Encoding" {
		t.Fatalf("json not gzipped: %v", rec.Header())
	}
	zr, _ := gzip.NewReader(rec.Body)
	if b, _ := io.ReadAll(zr); string(b) != json {
		t.Error("round trip mismatch")
	}
	if rec := req("/api/state", ""); rec.Header().Get("Content-Encoding") != "" || rec.Body.String() != json {
		t.Error("compressed without Accept-Encoding")
	}
	if rec := req("/api/x.png", "gzip"); rec.Header().Get("Content-Encoding") != "" {
		t.Error("image compressed")
	}
	for _, p := range []string{"/ws", "/go2rtc/api/x", "/frigate/api/x", "/api/printers/a/moonraker/server/files/list"} {
		if rec := req(p, "gzip"); rec.Header().Get("Content-Encoding") != "" {
			t.Errorf("%s must never be compressed", p)
		}
	}
	// Websocket upgrade is never wrapped.
	rec = do(h, "GET", "/api/state", func(r *http.Request) {
		r.Header.Set("Accept-Encoding", "gzip")
		r.Header.Set("Upgrade", "websocket")
	})
	if rec.Header().Get("Content-Encoding") != "" {
		t.Error("upgrade request compressed")
	}
}

func TestSecurityHeadersNotOnProxies(t *testing.T) {
	g, f := newUpstream(t), newUpstream(t)
	h := newRouter(t, "", g, f)
	if do(h, "GET", "/go2rtc/api/streams", nil).Header().Get("X-Frame-Options") != "" {
		t.Error("security headers leaked onto go2rtc proxy")
	}
	if do(h, "GET", "/api/version", nil).Header().Get("X-Frame-Options") != "SAMEORIGIN" {
		t.Error("missing headers on /api")
	}
}

type fakeResolver struct{ base, key string }

func (f *fakeResolver) PrinterTarget(id string) (string, string, bool) { return f.base, f.key, true }

func TestPrinterProxyStripsAndCaches(t *testing.T) {
	up := newUpstream(t)
	res := &fakeResolver{base: up.URL, key: "k1"}
	h := PrinterProxy(res, "tok")
	mux := http.NewServeMux()
	mux.Handle("/api/printers/{id}/moonraker/", h)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/printers/p1/moonraker/server/info?token=tok&x=1", nil)
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("Cookie", authCookie+"=zzz")
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("got %d", rec.Code)
	}
	l := up.last.Load()
	if l.Header.Get("Authorization") != "" || l.Header.Get("Cookie") != "" || l.URL.Query().Has("token") ||
		l.URL.Query().Get("x") != "1" || l.Header.Get("X-Api-Key") != "k1" || l.URL.Path != "/server/info" {
		t.Errorf("bad forwarded request: %v %v", l.Header, l.URL)
	}

	// Editing the key rebuilds the proxy; the cache holds one entry per printer.
	res.key = "k2"
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/printers/p1/moonraker/server/info", nil))
	if up.last.Load().Header.Get("X-Api-Key") != "k2" {
		t.Error("edited api key not picked up")
	}

	var cache sync.Map
	for _, key := range []string{"a", "b", "c"} {
		if _, err := printerProxyFor(&cache, "p1", up.URL, key, ""); err != nil {
			t.Fatal(err)
		}
	}
	n := 0
	cache.Range(func(_, _ any) bool { n++; return true })
	if n != 1 {
		t.Errorf("cache entries = %d, want 1", n)
	}
}
