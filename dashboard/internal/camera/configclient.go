package camera

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FrigateConfig edits Frigate's running config via its HTTP API (0.13+):
//
//	GET  /api/config/raw                         -> the raw YAML text
//	POST /api/config/save?save_option=saveonly   -> validate + write (no restart)
//	POST /api/config/save?save_option=restart    -> validate + write + restart
//
// It keeps a separate long-timeout client for the restart save (Frigate blocks
// ~25s) and writes a timestamped backup of the raw config before any change.
type FrigateConfig struct {
	baseURL   string
	backupDir string
	http      *http.Client // short ops: GetRaw / Validate / health
	saveHTTP  *http.Client // long op: SaveRestart (blocks during restart)
}

func NewFrigateConfig(baseURL, backupDir string) *FrigateConfig {
	return &FrigateConfig{
		baseURL:   strings.TrimRight(baseURL, "/"),
		backupDir: backupDir,
		http:      &http.Client{Timeout: 10 * time.Second},
		saveHTTP:  &http.Client{Timeout: 60 * time.Second},
	}
}

// GetRaw returns the raw config YAML text.
func (f *FrigateConfig) GetRaw(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.baseURL+"/api/config/raw", nil)
	if err != nil {
		return "", err
	}
	resp, err := f.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET /api/config/raw: HTTP %d", resp.StatusCode)
	}
	// Frigate returns the raw config as a JSON-encoded string (a quoted body with
	// \n escapes), not bare YAML — decode it back to the actual YAML text.
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil {
			return s, nil
		}
	}
	return string(body), nil
}

// Validate writes the config with save_option=saveonly: Frigate validates it and
// returns a 4xx with the validation message on failure, WITHOUT restarting.
func (f *FrigateConfig) Validate(ctx context.Context, raw string) error {
	return f.save(ctx, f.http, raw, "saveonly")
}

// SaveRestart writes the config and restarts Frigate (save_option=restart).
func (f *FrigateConfig) SaveRestart(ctx context.Context, raw string) error {
	return f.save(ctx, f.saveHTTP, raw, "restart")
}

func (f *FrigateConfig) save(ctx context.Context, cl *http.Client, raw, opt string) error {
	u := f.baseURL + "/api/config/save?save_option=" + opt
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader([]byte(raw)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain")
	resp, err := cl.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Frigate returns the Pydantic validation error in the body; surface it.
		return fmt.Errorf("%s", strings.TrimSpace(string(body)))
	}
	return nil
}

// backup writes a timestamped copy of raw into backupDir and returns its path.
func (f *FrigateConfig) backup(raw string) (string, error) {
	if f.backupDir == "" {
		return "", fmt.Errorf("no backup dir configured")
	}
	if err := os.MkdirAll(f.backupDir, 0o755); err != nil {
		return "", err
	}
	ts := time.Now().UTC().Format("20060102T150405Z")
	path := filepath.Join(f.backupDir, "frigate-config.backup."+ts+".yml")
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// WaitHealthy polls GET /api/config until it returns 200 or the timeout elapses
// (Frigate is briefly unreachable while restarting).
func (f *FrigateConfig) WaitHealthy(ctx context.Context, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	// Give the restart a moment to actually drop the old process first.
	select {
	case <-ctx.Done():
		return false
	case <-time.After(3 * time.Second):
	}
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.baseURL+"/api/config", nil)
		if err == nil {
			resp, err := f.http.Do(req)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return true
				}
			}
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(2 * time.Second):
		}
	}
	return false
}
