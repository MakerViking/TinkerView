package web

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
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

// spaFile is one embedded file with its precomputed validators and (for text-like
// types) gzip form. The embedded FS is immutable, so this is computed once.
type spaFile struct {
	ctype string
	etag  string // strong, quoted
	raw   []byte
	gz    []byte // nil when not compressible / not smaller
	cache string // Cache-Control
}

func newSPAFile(name string, raw []byte, cache string) *spaFile {
	ct := mime.TypeByExtension(path.Ext(name))
	if ct == "" {
		ct = http.DetectContentType(raw)
	}
	sum := sha256.Sum256(raw)
	f := &spaFile{ctype: ct, raw: raw, cache: cache, etag: `"` + hex.EncodeToString(sum[:8]) + `"`}
	if len(raw) >= 256 && compressibleType(ct) {
		var buf bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
		_, _ = zw.Write(raw)
		if zw.Close() == nil && buf.Len() < len(raw) {
			f.gz = buf.Bytes()
		}
	}
	return f
}

// SPAHandler serves the embedded SPA: a real file when present, else index.html
// (client-side routing fallback — also for directories like /assets/, which must not
// produce a file listing). Hashed /assets/* are immutable-cached; everything else
// (index.html, video-rtc.js, icons) is served with ETag + no-cache so a deploy takes
// effect immediately but unchanged files cost only a 304. Text-like files are served
// gzipped (precomputed) when the client accepts it.
func SPAHandler() (http.Handler, error) {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil, err
	}
	files := map[string]*spaFile{}
	err = fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := fs.ReadFile(sub, p)
		if err != nil {
			return err
		}
		cache := "no-cache"
		if strings.HasPrefix(p, "assets/") {
			cache = "public, max-age=31536000, immutable"
		}
		files[p] = newSPAFile(p, b, cache)
		return nil
	})
	if err != nil {
		return nil, err
	}
	// index.html must never be long-cached: it references hash-named bundles, and a
	// stale copy would keep loading the OLD bundle after a deploy.
	if b, ok := files["index.html"]; ok {
		files["index.html"] = newSPAFile("index.html", b.raw, "no-cache")
	} else {
		files["index.html"] = newSPAFile("index.html", []byte(placeholderHTML), "no-cache")
	}
	index := files["index.html"]
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		f, ok := files[strings.TrimPrefix(r.URL.Path, "/")]
		if !ok {
			f = index // SPA fallback (unknown path or directory)
		}
		serveSPAFile(w, r, f)
	}), nil
}

func serveSPAFile(w http.ResponseWriter, r *http.Request, f *spaFile) {
	h := w.Header()
	h.Set("Content-Type", f.ctype)
	h.Set("Cache-Control", f.cache)
	body := f.raw
	etag := f.etag
	if f.gz != nil {
		h.Add("Vary", "Accept-Encoding")
		// Range over a gzip representation is pointless here; serve identity then.
		if acceptsGzip(r) && r.Header.Get("Range") == "" {
			body = f.gz
			etag = strings.TrimSuffix(f.etag, `"`) + `-gz"`
			h.Set("Content-Encoding", "gzip")
		}
	}
	h.Set("ETag", etag)
	// Zero modtime: validators come from the ETag only (embed.FS has no mtimes).
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(body))
}
