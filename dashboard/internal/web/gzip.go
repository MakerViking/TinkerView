package web

import (
	"compress/gzip"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// acceptsGzip reports whether the client's Accept-Encoding permits gzip.
func acceptsGzip(r *http.Request) bool {
	for _, v := range r.Header.Values("Accept-Encoding") {
		for _, part := range strings.Split(v, ",") {
			name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
			if !strings.EqualFold(strings.TrimSpace(name), "gzip") {
				continue
			}
			params = strings.TrimSpace(params)
			if q, ok := strings.CutPrefix(params, "q="); ok {
				if f, err := strconv.ParseFloat(strings.TrimSpace(q), 64); err == nil && f == 0 {
					return false
				}
			}
			return true
		}
	}
	return false
}

// compressibleType reports whether a Content-Type is worth gzipping.
func compressibleType(ct string) bool {
	ct, _, _ = strings.Cut(ct, ";")
	ct = strings.ToLower(strings.TrimSpace(ct))
	switch {
	case strings.HasPrefix(ct, "text/"):
		return true
	case ct == "application/json", ct == "application/javascript", ct == "application/xml",
		ct == "image/svg+xml", ct == "application/manifest+json":
		return true
	}
	return false
}

var gzipPool = sync.Pool{New: func() any { return gzip.NewWriter(nil) }}

// gzipWriter compresses a response iff, at header time, its Content-Type is text-like,
// it isn't already encoded and isn't a partial/empty response.
type gzipWriter struct {
	http.ResponseWriter
	req     *http.Request
	decided bool
	gz      *gzip.Writer
}

func (g *gzipWriter) decide(status int) {
	if g.decided {
		return
	}
	g.decided = true
	h := g.Header()
	if !compressibleType(h.Get("Content-Type")) {
		return
	}
	if status < 200 || status == http.StatusNoContent || status == http.StatusNotModified ||
		h.Get("Content-Encoding") != "" || h.Get("Content-Range") != "" ||
		g.req.Method == http.MethodHead {
		return
	}
	h.Del("Content-Length")
	h.Set("Content-Encoding", "gzip")
	g.gz = gzipPool.Get().(*gzip.Writer)
	g.gz.Reset(g.ResponseWriter)
}

func (g *gzipWriter) WriteHeader(code int) {
	g.decide(code)
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipWriter) Write(b []byte) (int, error) {
	g.decide(http.StatusOK)
	if g.gz != nil {
		return g.gz.Write(b)
	}
	return g.ResponseWriter.Write(b)
}

func (g *gzipWriter) Flush() {
	if g.gz != nil {
		_ = g.gz.Flush()
	}
	_ = http.NewResponseController(g.ResponseWriter).Flush()
}

func (g *gzipWriter) Unwrap() http.ResponseWriter { return g.ResponseWriter }

func (g *gzipWriter) close() {
	if g.gz != nil {
		_ = g.gz.Close()
		g.gz.Reset(nil)
		gzipPool.Put(g.gz)
		g.gz = nil
	}
}

// withGzip compresses text-like /api/ responses (JSON) for clients that accept it.
// The printer Moonraker proxy, /ws, and the go2rtc/Frigate proxies are never
// compressed (streams, binaries, WebSocket upgrades); the SPA serves its own
// precompressed assets.
func withGzip(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if !strings.HasPrefix(p, "/api/") || strings.Contains(p, "/moonraker/") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Add("Vary", "Accept-Encoding")
		if !acceptsGzip(r) || r.Header.Get("Upgrade") != "" || r.Header.Get("Range") != "" {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipWriter{ResponseWriter: w, req: r}
		defer gw.close()
		next.ServeHTTP(gw, r)
	})
}
