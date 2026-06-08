package web

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// ReverseProxy proxies requests under pathPrefix to target, stripping the prefix
// (e.g. /go2rtc/api/ws -> /api/ws upstream). httputil.ReverseProxy transparently
// handles the WebSocket Upgrade on HTTP/1.1, which is what go2rtc's /api/ws needs.
//
// stripCrossOrigin controls how browser cross-origin headers are handled:
//   - false (go2rtc): rewrite Origin to the upstream's own origin so go2rtc's
//     same-origin WebSocket check (/api/ws) passes.
//   - true (Frigate): DELETE Origin and Referer entirely. Frigate's CSRF guard
//     rejects any mutating request (POST/PUT/DELETE — e.g. enrolling a face) that
//     carries an Origin header, even when it equals Frigate's own origin; a request
//     with no Origin is treated as a trusted non-browser caller and allowed. This
//     is what lets the Recognition tab's face/plate writes succeed through the proxy.
func ReverseProxy(pathPrefix, target string, stripCrossOrigin bool) (http.Handler, error) {
	u, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	rp := httputil.NewSingleHostReverseProxy(u)
	orig := rp.Director
	rp.Director = func(r *http.Request) {
		orig(r)
		r.URL.Path = strings.TrimPrefix(r.URL.Path, pathPrefix)
		if !strings.HasPrefix(r.URL.Path, "/") {
			r.URL.Path = "/" + r.URL.Path
		}
		r.Host = u.Host
		// SAFE because upstreams (go2rtc/Frigate) are internal/trusted and reached
		// only over the tailnet; the dashboard is the gatekeeper. Do NOT point this
		// at an upstream that relies solely on an Origin check to gate state-changing
		// endpoints.
		if stripCrossOrigin {
			r.Header.Del("Origin")
			r.Header.Del("Referer")
		} else if r.Header.Get("Origin") != "" {
			r.Header.Set("Origin", u.Scheme+"://"+u.Host)
		}
	}
	// Log upstream failures (e.g. go2rtc/Frigate down) in the dashboard's own log.
	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Warn("upstream proxy error", "target", target, "path", r.URL.Path, "err", err.Error())
		w.WriteHeader(http.StatusBadGateway)
	}
	return rp, nil
}
