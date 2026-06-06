package web

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// dist holds the built React SPA. A placeholder index.html is committed so the
// Go build always succeeds; the Docker build overwrites this dir with the real
// Vite output (COPY --from=web).
//
//go:embed all:dist
var dist embed.FS

const placeholderHTML = `<!doctype html><html><head><meta charset="utf-8">
<title>TinkerView</title></head><body style="font-family:system-ui;background:#0b0f14;color:#cbd5e1;padding:2rem">
<h1>TinkerView</h1><p>The frontend has not been built into this binary yet.
Run <code>pnpm build</code> in <code>dashboard/web</code> (or build the Docker image, which does it for you).</p>
<p>Backend API is live: <a style="color:#38bdf8" href="/api/version">/api/version</a> ·
<a style="color:#38bdf8" href="/healthz">/healthz</a></p></body></html>`

// SPAHandler serves the embedded SPA: a real asset when present, else index.html
// (client-side routing fallback).
func SPAHandler() (http.Handler, error) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, err
	}
	fileServer := http.FileServer(http.FS(sub))
	index, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		index = []byte(placeholderHTML)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			serveHTML(w, index)
			return
		}
		if f, err := sub.Open(p); err == nil {
			_ = f.Close()
			fileServer.ServeHTTP(w, r)
			return
		}
		serveHTML(w, index) // SPA fallback
	}), nil
}

func serveHTML(w http.ResponseWriter, b []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b)
}
