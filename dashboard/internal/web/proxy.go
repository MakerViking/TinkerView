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
func ReverseProxy(pathPrefix, target string) (http.Handler, error) {
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
		// Make the upstream's same-origin WebSocket check pass (go2rtc's /api/ws
		// returns 403 otherwise): rewrite Origin to the upstream's own origin.
		// SAFE because upstreams (go2rtc/Frigate) are internal/trusted and reached
		// only over the tailnet; state-changing actions go through the dashboard's
		// own authenticated WS, not this proxy. Do NOT proxy an upstream that
		// relies solely on an Origin check to gate state-changing endpoints.
		if r.Header.Get("Origin") != "" {
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
