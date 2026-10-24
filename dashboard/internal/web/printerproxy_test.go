package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type allowlistResolver struct{ base string }

func (f allowlistResolver) PrinterTarget(id string) (string, string, bool) {
	if id != "p1" {
		return "", "", false
	}
	return f.base, "secret-key", true
}

func TestPrinterProxyAllowlist(t *testing.T) {
	var gotKey, gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey, gotPath = r.Header.Get("X-Api-Key"), r.URL.Path
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	mux := http.NewServeMux()
	mux.Handle("/api/printers/{id}/moonraker/", PrinterProxy(allowlistResolver{upstream.URL}, ""))

	cases := []struct {
		path string
		want int
	}{
		{"/api/printers/p1/moonraker/server/files/list?root=gcodes", http.StatusOK},
		{"/api/printers/p1/moonraker/server/files/metadata?filename=a.gcode", http.StatusOK},
		{"/api/printers/p1/moonraker/server/history/list", http.StatusOK},
		{"/api/printers/p1/moonraker/server/history/totals", http.StatusOK},
		{"/api/printers/p1/moonraker/server/files/gcodes/.thumbs/a-300x300.png", http.StatusOK},
		{"/api/printers/p1/moonraker/access/api_key", http.StatusForbidden},
		{"/api/printers/p1/moonraker/access/oneshot_token", http.StatusForbidden},
		{"/api/printers/p1/moonraker/server/files/config/printer.cfg", http.StatusForbidden},
		{"/api/printers/p1/moonraker/machine/system_info", http.StatusForbidden},
		{"/api/printers/p1/moonraker/printer/gcode/script?script=G28", http.StatusForbidden},
		{"/api/printers/p1/moonraker/server/files/gcodes/%2e%2e/%2e%2e/access/api_key", http.StatusForbidden},
		{"/api/printers/nope/moonraker/server/files/list", http.StatusNotFound},
	}
	for _, c := range cases {
		gotKey, gotPath = "", ""
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, c.path, nil))
		if rec.Code != c.want {
			t.Errorf("%s: status %d, want %d (upstream saw %q)", c.path, rec.Code, c.want, gotPath)
			continue
		}
		if c.want == http.StatusOK && gotKey != "secret-key" {
			t.Errorf("%s: upstream did not receive the API key", c.path)
		}
		if c.want != http.StatusOK && gotPath != "" {
			t.Errorf("%s: request reached upstream as %q", c.path, gotPath)
		}
	}
}
