package web

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// statusRecorder captures the response status for logging. It implements
// Unwrap() so http.ResponseController (and thus coder/websocket's Accept and
// httputil.ReverseProxy's WS upgrade) can still reach the underlying Hijacker/Flusher.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wrote {
		r.status = code
		r.wrote = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.wrote = true
	return r.ResponseWriter.Write(b)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// responseStarted reports whether headers/body were already sent on w, looking
// through Unwrap() wrappers to the logging recorder.
func responseStarted(w http.ResponseWriter) bool {
	for {
		if sr, ok := w.(*statusRecorder); ok {
			return sr.wrote
		}
		u, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return false
		}
		w = u.Unwrap()
	}
}

// withLogging logs each request (2xx at debug to avoid spam, 4xx warn, 5xx error)
// and recovers panics so one bad handler can't crash the process — the panic is
// logged with a stack trace.
func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		defer func() {
			if rv := recover(); rv != nil {
				slog.Error("panic in HTTP handler",
					"method", r.Method, "path", r.URL.Path, "panic", rv,
					"stack", string(debug.Stack()))
				func() {
					defer func() { _ = recover() }() // ignore if already hijacked/written
					http.Error(rec, "internal server error", http.StatusInternalServerError)
				}()
			}
			lvl := slog.LevelDebug
			switch {
			case rec.status >= 500:
				lvl = slog.LevelError
			case rec.status >= 400:
				lvl = slog.LevelWarn
			}
			slog.Log(r.Context(), lvl, "http",
				"method", r.Method, "path", r.URL.Path, "status", rec.status,
				"dur", time.Since(start).Round(time.Millisecond).String())
		}()
		next.ServeHTTP(rec, r)
	})
}
