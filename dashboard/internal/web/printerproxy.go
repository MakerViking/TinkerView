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
// token is the dashboard's shared token ("" = none): it is stripped (with the
// Authorization header and cookies) before forwarding to Moonraker.
func PrinterProxy(resolver TargetResolver, token string) http.Handler {
	var cache sync.Map // printer id -> *printerProxy
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
		rp, err := printerProxyFor(&cache, id, base, apiKey, token)
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

// printerProxy is a cached reverse proxy plus the target it was built for.
type printerProxy struct {
	base, apiKey string
	rp           *httputil.ReverseProxy
}

// printerProxyFor returns the proxy for a printer, rebuilding it when its base URL
// or API key changed. Keyed by printer id (not base+key) so edits replace the entry
// instead of leaking one per distinct value.
func printerProxyFor(cache *sync.Map, id, base, apiKey, token string) (*httputil.ReverseProxy, error) {
	if v, ok := cache.Load(id); ok {
		if pp := v.(*printerProxy); pp.base == base && pp.apiKey == apiKey {
			return pp.rp, nil
		}
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
		// The dashboard's credentials are never meant for Moonraker.
		stripCredentials(req, token)
		req.Header.Del("Authorization")
		req.Header.Del("Cookie")
		if apiKey != "" {
			req.Header.Set("X-Api-Key", apiKey)
		}
	}
	rp.ErrorHandler = proxyErrorHandler(u.Redacted())
	cache.Store(id, &printerProxy{base: base, apiKey: apiKey, rp: rp})
	return rp, nil
}
