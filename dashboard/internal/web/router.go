package web

import (
	"net/http"

	"github.com/MakerViking/TinkerView/internal/config"
	"github.com/MakerViking/TinkerView/internal/hub"
)

// Router builds the top-level handler. API/WS/proxy prefixes are registered so
// they match before the SPA fallback ("/") — otherwise API 404s would be masked
// by index.html.
func Router(cfg *config.Config, h *hub.Hub, api *API, resolver TargetResolver) (http.Handler, error) {
	mux := http.NewServeMux()

	mux.HandleFunc("/ws", h.ServeWS)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	api.Register(mux)
	// Read-only per-printer Moonraker proxy (file browse / thumbnails / history).
	if resolver != nil {
		mux.Handle("/api/printers/{id}/moonraker/", PrinterProxy(resolver, cfg.Auth.Token))
	}
	// Ensure unregistered /api/* returns a real 404 instead of being swallowed by
	// the SPA fallback at "/". The specific "GET /api/..." routes above take
	// precedence over this subtree pattern.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})

	if cfg.Go2RTCURL != "" {
		gp, err := ReverseProxy("/go2rtc", cfg.Go2RTCURL, false, cfg.Auth.Token) // rewrite Origin (WebSocket)
		if err != nil {
			return nil, err
		}
		// Read-only: the SPA only GETs (api/ws, stream.mjpeg, frame.jpeg); go2rtc's
		// write endpoints (config, streams PUT, exec sources) must not be reachable.
		mux.Handle("/go2rtc/", readOnlyMethods(gp))
	}
	if cfg.Frigate.BaseURL != "" {
		fp, err := ReverseProxy("/frigate", cfg.Frigate.BaseURL, true, cfg.Auth.Token) // strip Origin (Frigate CSRF guard)
		if err != nil {
			return nil, err
		}
		mux.Handle("/frigate/", fp)
	}

	spa, err := SPAHandler()
	if err != nil {
		return nil, err
	}
	mux.Handle("/", spa)

	// Outermost: request logging + panic recovery, then security headers, the
	// CSRF/body guards, auth (gates /api/ and the media proxies), gzip for JSON
	// /api/ responses, then the mux.
	return withLogging(withSecurityHeaders(withRequestGuards(withAuth(cfg.Auth.Token, withGzip(mux))))), nil
}
