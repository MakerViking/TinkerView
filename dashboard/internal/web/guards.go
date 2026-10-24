package web

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

const (
	// authCookie carries a hash of the shared token so <img>/<video>/WebSocket
	// requests to the media proxies (which can't set headers, and which the SPA does
	// not decorate with ?token=) authenticate without the raw token ever being put
	// in a URL or forwarded upstream. SameSite=Strict + HttpOnly; it is accepted ONLY
	// for the proxies, never for /api/.
	authCookie = "tv_auth"

	// maxAPIBody bounds request bodies on /api/* (all handlers decode small JSON).
	maxAPIBody = 1 << 20
)

// tokenEqual compares in constant time (hashing first so length doesn't leak).
func tokenEqual(a, b string) bool {
	ha, hb := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

func cookieValue(token string) string {
	sum := sha256.Sum256([]byte("tinkerview-auth\x00" + token))
	return hex.EncodeToString(sum[:])
}

// requestToken extracts a token presented via Authorization: Bearer or ?token=.
func requestToken(r *http.Request) string {
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if got == "" {
		got = r.URL.Query().Get("token")
	}
	return got
}

func hasValidCookie(token string, r *http.Request) bool {
	c, err := r.Cookie(authCookie)
	return err == nil && tokenEqual(c.Value, cookieValue(token))
}

func isProxyPath(p string) bool {
	return strings.HasPrefix(p, "/go2rtc/") || strings.HasPrefix(p, "/frigate/")
}

// withAuth gates /api/* and the /go2rtc/ + /frigate/ proxies with the shared token
// when one is configured. /api/ needs the header or ?token=; the proxies also accept
// the auth cookie (set on any request that presents a valid token) because their
// consumers (<img>, <video>, WebSocket) can't send headers. Static assets and /ws
// (which self-checks ?token=) stay open so the login page loads.
func withAuth(token string, next http.Handler) http.Handler {
	if token == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		presented := requestToken(r)
		valid := presented != "" && tokenEqual(presented, token)
		if valid && !hasValidCookie(token, r) {
			http.SetCookie(w, &http.Cookie{
				Name: authCookie, Value: cookieValue(token), Path: "/",
				MaxAge: 30 * 24 * 3600, HttpOnly: true, SameSite: http.SameSiteStrictMode,
			})
		}
		switch {
		case strings.HasPrefix(path, "/api/"):
			if !valid {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		case isProxyPath(path):
			if !valid && !hasValidCookie(token, r) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// stripCredentials removes the dashboard's own credentials from a request before it
// is forwarded to an upstream (so they don't land in upstream/access logs). token is
// the configured shared token ("" = none configured, nothing to strip).
func stripCredentials(r *http.Request, token string) {
	if token == "" {
		return
	}
	r.Header.Del("Authorization")
	if q := r.URL.Query(); q.Has("token") {
		q.Del("token")
		r.URL.RawQuery = q.Encode()
	}
	if cs := r.Cookies(); len(cs) > 0 {
		r.Header.Del("Cookie")
		for _, c := range cs {
			if c.Name != authCookie {
				r.AddCookie(c)
			}
		}
	}
}

func mutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// sameOrigin reports whether an Origin header value names this server. Behind a
// reverse proxy that preserves Host (Caddy) Origin == Host; X-Forwarded-Host is
// accepted too for proxies that rewrite it.
func sameOrigin(origin string, r *http.Request) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false // includes "null"
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	if xf := r.Header.Get("X-Forwarded-Host"); xf != "" && strings.EqualFold(u.Host, xf) {
		return true
	}
	return false
}

// withRequestGuards applies, to mutating requests under /api/, /go2rtc/ and
// /frigate/: a CSRF Origin check (reject when an Origin is present and isn't us;
// an absent Origin is allowed for curl/scripts), and for /api/ a request-body cap.
// The Frigate proxy strips Origin before forwarding (its own CSRF guard), so this is
// the only cross-site protection those writes get.
func withRequestGuards(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mutating(r.Method) {
			p := r.URL.Path
			isAPI := strings.HasPrefix(p, "/api/")
			if isAPI || isProxyPath(p) {
				if o := r.Header.Get("Origin"); o != "" && !sameOrigin(o, r) {
					slog.Warn("cross-origin request rejected", "method", r.Method, "path", p, "origin", o)
					http.Error(w, "cross-origin request forbidden", http.StatusForbidden)
					return
				}
			}
			if isAPI && r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, maxAPIBody)
			}
		}
		next.ServeHTTP(w, r)
	})
}

// withSecurityHeaders sets conservative headers on the SPA and /api responses (not
// the proxied go2rtc/Frigate content). Deliberately no CSP: it risks breaking the
// go2rtc WebRTC/MSE player.
func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isProxyPath(r.URL.Path) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "SAMEORIGIN")
			h.Set("Referrer-Policy", "same-origin")
		}
		next.ServeHTTP(w, r)
	})
}

// readOnlyMethods lets only GET/HEAD through (go2rtc's write endpoints — config,
// streams PUT, exec sources — must never be reachable through the dashboard proxy;
// the SPA only reads: api/ws, stream.mjpeg, frame.jpeg). It also requires every
// src= to be a bare configured stream name: go2rtc creates an ad-hoc stream from
// any src containing a scheme (rtsp://..., exec:...), which a GET could otherwise
// trigger. The SPA only ever passes camera names (which cannot contain ':').
func readOnlyMethods(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		for _, v := range r.URL.Query()["src"] {
			if strings.Contains(v, ":") {
				http.Error(w, "ad-hoc stream sources are not allowed", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
