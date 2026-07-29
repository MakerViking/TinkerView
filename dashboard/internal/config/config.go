package config

import (
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// defaultDataDir is the writable directory the dashboard owns, mounted rw in
// compose. The config file lives here too (see Resolve).
const defaultDataDir = "/data"

// legacyConfigPath is where installs before the move mounted config.yaml, as a
// single-file bind mount. Still read so those stacks keep working untouched.
const legacyConfigPath = "/config.yaml"

// templateYAML is an entirely commented-out config, written to the primary path
// on first run so the file is discoverable without a trip to GitHub.
//
//go:embed template.yaml
var templateYAML []byte

// Config is the whole dashboard configuration. Loaded from YAML, with a few env
// overrides for the secrets/paths that vary per host. Restart to apply.
type Config struct {
	Listen     string      `yaml:"listen"`
	Log        Log         `yaml:"log"`
	Auth       Auth        `yaml:"auth"`
	Frigate    Frigate     `yaml:"frigate"`
	Go2RTCURL  string      `yaml:"go2rtc_url"`
	Docker     Docker      `yaml:"docker"`
	Ntfy       Ntfy        `yaml:"ntfy"`
	Printers   []Printer   `yaml:"printers"`
	QuickLinks []QuickLink `yaml:"quick_links"`
	// DataDir is a writable directory the dashboard owns: it holds UI-managed
	// devices (devices.yaml), Frigate-config backups, and — since the move off a
	// single-file bind mount — config.yaml itself. Defaults to /data (mounted rw).
	DataDir string `yaml:"data_dir"`
	// HardwareTranscode, when true, appends go2rtc's #hardware hint to transcoded
	// (MJPEG/raw) camera sources added from the UI so the host GPU (e.g. NVENC) is
	// used. Leave false on CPU-only hosts. Kept off by default so the repo stays
	// host-agnostic; the tower config sets it true.
	HardwareTranscode bool `yaml:"hardware_transcode"`
}

type Log struct {
	Level string `yaml:"level"` // debug | info | warn | error  (default info)
	File  string `yaml:"file"`  // optional: also append logs to this path (stdout always on)
}

type Auth struct {
	// Token, if set, gates the REST API (Authorization: Bearer) and the WS
	// handshake (?token=). Empty = open (rely on the Tailscale perimeter).
	Token string `yaml:"token"`
}

type Frigate struct {
	BaseURL     string `yaml:"base_url"`     // http://127.0.0.1:5000
	MQTTAddr    string `yaml:"mqtt_addr"`    // tcp://127.0.0.1:1883
	TopicPrefix string `yaml:"topic_prefix"` // frigate
	MediaPath   string `yaml:"media_path"`   // /media/frigate (for Statfs)
}

type Docker struct {
	Enabled bool     `yaml:"enabled"`
	Host    string   `yaml:"host"`  // tcp://127.0.0.1:2375 (docker-socket-proxy) or unix:///var/run/docker.sock
	Watch   []string `yaml:"watch"` // container names to surface/allow-restart
}

type Ntfy struct {
	BaseURL string `yaml:"base_url"`
	Topic   string `yaml:"topic"`
	Token   string `yaml:"token"`
}

// Printer is one configured printer. Fields used depend on Kind:
//
//	moonraker: base_url (http://ip:7125), optional api_key
//	octoprint: base_url (http://ip), api_key
//	bambu:     ip, serial, access_code, model
type Printer struct {
	ID         string   `yaml:"id"`
	Kind       string   `yaml:"kind"`
	Name       string   `yaml:"name"`
	BaseURL    string   `yaml:"base_url"`
	APIKey     string   `yaml:"api_key"`
	IP         string   `yaml:"ip"`
	Serial     string   `yaml:"serial"`
	AccessCode string   `yaml:"access_code"`
	Model      string   `yaml:"model"`
	Camera     string   `yaml:"camera"` // go2rtc stream name for this printer's cam
	UIURL      string   `yaml:"ui_url"` // web UI to open on click; defaults to base_url
	Macros     []string `yaml:"macros"` // curated gcode macros exposed as UI buttons (e.g. T0..T3, M600)
}

type QuickLink struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// candidatePaths lists the config locations to try, most-preferred first.
func candidatePaths() []string {
	dir := os.Getenv("DASHBOARD_DATA_DIR")
	if dir == "" {
		dir = defaultDataDir
	}
	return []string{filepath.Join(dir, "config.yaml"), legacyConfigPath}
}

// Resolve picks the config file to load. An explicit path (the -config flag) or
// DASHBOARD_CONFIG wins outright; otherwise the first candidate that is a readable
// regular FILE is used, and if there is none the primary path is returned so logs
// and SeedTemplate name the place to create one.
//
// The primary location lives inside the data DIRECTORY mount on purpose. Bind-
// mounting a single file that does not exist on the host makes Docker create a
// directory in its place, which was the most common first-run failure: the
// dashboard then started with no config and the path could not be fixed by
// copying a file over it. Only regular files are accepted here, so a leftover
// directory at either candidate is skipped rather than loaded.
func Resolve(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if v := os.Getenv("DASHBOARD_CONFIG"); v != "" {
		return v
	}
	paths := candidatePaths()
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
			return p
		}
	}
	return paths[0]
}

// SeedTemplate writes an annotated, fully commented-out config to path when
// nothing exists there, so a fresh install has an editable file at a known
// location. Best-effort: a read-only mount or an unwritable data dir is not an
// error — the dashboard runs on defaults with no config file at all, and devices
// can be added from the UI.
func SeedTemplate(path string) {
	if _, err := os.Stat(path); err == nil || !os.IsNotExist(err) {
		return // exists already (file OR leftover directory) — never overwrite
	}
	if err := os.WriteFile(path, templateYAML, 0o644); err != nil {
		slog.Debug("could not write config template", "path", path, "err", err)
		return
	}
	slog.Info("wrote a commented config template", "path", path)
}

// Load reads YAML (if path is non-empty and exists) over sensible defaults, then
// applies env overrides.
func Load(path string) (*Config, error) {
	c := &Config{
		Listen:    ":8088",
		Log:       Log{Level: "info"},
		DataDir:   defaultDataDir,
		Go2RTCURL: "http://127.0.0.1:1984",
		Frigate: Frigate{
			BaseURL:     "http://127.0.0.1:5000",
			MQTTAddr:    "tcp://127.0.0.1:1883",
			TopicPrefix: "frigate",
			MediaPath:   "/media/frigate",
		},
		Docker: Docker{Enabled: true, Host: "tcp://127.0.0.1:2375"},
	}
	if path != "" {
		if b, err := os.ReadFile(path); err == nil {
			if err := yaml.Unmarshal(b, c); err != nil {
				return nil, fmt.Errorf("parse config %s: %w", path, err)
			}
		} else if os.IsNotExist(err) {
			slog.Info("no config file; starting with defaults — add printers/cameras from the UI", "path", path)
		} else {
			// Unreadable for some reason other than "missing" — most commonly a
			// directory that Docker auto-creates when a bind-mounted config FILE
			// doesn't exist on the host (read returns EISDIR, not IsNotExist). Don't
			// crash: warn and run on defaults — the UI and the writable device store
			// still work. A present-but-malformed file is handled above (fatal).
			reason := err.Error()
			if st, serr := os.Stat(path); serr == nil && st.IsDir() {
				reason = "path is a directory (likely a bind-mount of a config file that doesn't exist on the host)"
			}
			slog.Warn("config not readable; starting with defaults — add printers/cameras from the UI", "path", path, "reason", reason)
		}
	}
	if v := os.Getenv("DASHBOARD_TOKEN"); v != "" {
		c.Auth.Token = v
	}
	if v := os.Getenv("DASHBOARD_LISTEN"); v != "" {
		c.Listen = v
	}
	if v := os.Getenv("DASHBOARD_DATA_DIR"); v != "" {
		c.DataDir = v
	}
	if v := os.Getenv("DASHBOARD_LOG_LEVEL"); v != "" {
		c.Log.Level = v
	}
	if v := os.Getenv("DOCKER_HOST"); v != "" {
		c.Docker.Host = v
	}
	if v := os.Getenv("NTFY_BASE_URL"); v != "" {
		c.Ntfy.BaseURL = v
	}
	if v := os.Getenv("NTFY_TOKEN"); v != "" {
		c.Ntfy.Token = v
	}
	return c, nil
}
