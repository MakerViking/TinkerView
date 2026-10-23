package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/MakerViking/TinkerView/internal/config"
)

// healthcheck probes the running dashboard's /healthz and returns a process exit
// code (0 healthy, 1 not). It exists because the runtime image is distroless:
// there is no shell, curl or wget for a Docker HEALTHCHECK to call, so the binary
// checks itself. It resolves the listen address exactly like the server does
// (config file, then DASHBOARD_LISTEN), so a custom port keeps working.
func healthcheck(configPath string) int {
	// config.Load logs at info level; keep the probe silent unless it fails.
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	cfg, err := config.Load(config.Resolve(configPath))
	slog.SetDefault(prev)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck: load config:", err)
		return 1
	}
	url := "http://" + probeAddr(cfg.Listen) + "/healthz"

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: unexpected status", resp.Status)
		return 1
	}
	return 0
}

// probeAddr turns a listen address (":8088", "0.0.0.0:8088", "[::]:8088",
// "10.0.0.5:8088") into a host:port the probe can dial on the same machine.
func probeAddr(listen string) string {
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return "127.0.0.1:8088"
	}
	switch host {
	case "", "0.0.0.0", "::":
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

// warnIfOpen logs one clear warning when no auth token is configured. Auth
// behaviour is unchanged (an empty token means open access); this only makes the
// consequence visible in `docker logs`.
func warnIfOpen(cfg *config.Config) {
	if cfg.Auth.Token != "" {
		return
	}
	slog.Warn(fmt.Sprintf("dashboard is open to anyone who can reach port %s; set auth.token or DASHBOARD_TOKEN - see SECURITY.md", listenPort(cfg.Listen)))
}

func listenPort(listen string) string {
	if _, port, err := net.SplitHostPort(listen); err == nil && port != "" {
		return port
	}
	return listen
}
