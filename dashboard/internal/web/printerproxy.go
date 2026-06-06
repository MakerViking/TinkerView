package web

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
)

// TargetResolver maps a printer id to its Moonraker base URL + API key. Implemented
// by the device manager.
type TargetResolver interface {
	PrinterTarget(id string) (baseURL, apiKey string, ok bool)
}

// PrinterProxy serves GET/HEAD /api/printers/{id}/moonraker/* by reverse-proxying to
// that printer's Moonraker (injecting its X-Api-Key server-side), so the frontend can
// browse files, thumbnails, metadata, and history same-origin (Moonraker CORS would
// block a direct cross-origin fetch). It is READ-ONLY: state-changing actions go over
// the authenticated WS Control path, never this proxy. Lives under /api/ so the bearer
// auth applies; <img> thumbnails use the ?token= query fallback withAuth supports.
func PrinterProxy(resolver TargetResolver) http.Handler {
	var cache sync.Map // base+key -> *httputil.ReverseProxy
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := r.PathValue("id")
		base, apiKey, ok := resolver.PrinterTarget(id)
		if !ok {
			http.Error(w, "unknown printer", http.StatusNotFound)
			return
		}
		rp, err := printerProxyFor(&cache, base, apiKey)
		if err != nil {
			http.Error(w, "bad printer target", http.StatusBadGateway)
			return
		}
		// Strip /api/printers/{id}/moonraker, leaving the Moonraker path (e.g.
		// /server/files/list); the query string is preserved on r.URL.
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api/printers/"+id+"/moonraker")
		if !strings.HasPrefix(r.URL.Path, "/") {
			r.URL.Path = "/" + r.URL.Path
		}
		// Clear RawPath so EscapedPath re-derives from the (stripped) Path — keeps
		// percent-encoded filenames consistent after the prefix trim.
		r.URL.RawPath = ""
		rp.ServeHTTP(w, r)
	})
}

func printerProxyFor(cache *sync.Map, base, apiKey string) (*httputil.ReverseProxy, error) {
	key := base + "\x00" + apiKey
	if v, ok := cache.Load(key); ok {
		return v.(*httputil.ReverseProxy), nil
	}
	u, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	rp := httputil.NewSingleHostReverseProxy(u)
	orig := rp.Director
	rp.Director = func(req *http.Request) {
		orig(req)
		req.Host = u.Host
		if apiKey != "" {
			req.Header.Set("X-Api-Key", apiKey)
		}
	}
	cache.Store(key, rp)
	return rp, nil
}
