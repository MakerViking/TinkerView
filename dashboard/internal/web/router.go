package web

import (
	"net/http"
	"strings"

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
		mux.Handle("/api/printers/{id}/moonraker/", PrinterProxy(resolver))
	}
	// Ensure unregistered /api/* returns a real 404 instead of being swallowed by
	// the SPA fallback at "/". The specific "GET /api/..." routes above take
	// precedence over this subtree pattern.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	})

	if cfg.Go2RTCURL != "" {
		gp, err := ReverseProxy("/go2rtc", cfg.Go2RTCURL)
		if err != nil {
			return nil, err
		}
		mux.Handle("/go2rtc/", gp)
	}
	if cfg.Frigate.BaseURL != "" {
		fp, err := ReverseProxy("/frigate", cfg.Frigate.BaseURL)
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

	// Outermost: request logging + panic recovery, then auth, then the mux.
	return withLogging(withAuth(cfg.Auth.Token, mux)), nil
}

// withAuth gates /api/* with a bearer token when one is configured. Static assets,
// /ws (self-checks via ?token=), and the media proxies stay open so the login page
// loads and <img>/<video> elements (which can't set headers) work over the tailnet.
func withAuth(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if got == "" {
				got = r.URL.Query().Get("token")
			}
			if got != token {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
