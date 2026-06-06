package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

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
	// DataDir is a writable directory the dashboard owns (separate from the
	// read-only config.yaml mount): it holds UI-managed devices (devices.yaml)
	// and Frigate-config backups. Defaults to /data (mounted rw in compose).
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

// Load reads YAML (if path is non-empty and exists) over sensible defaults, then
// applies env overrides.
func Load(path string) (*Config, error) {
	c := &Config{
		Listen:    ":8088",
		Log:       Log{Level: "info"},
		DataDir:   "/data",
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
				return nil, err
			}
		} else if !os.IsNotExist(err) {
			return nil, err
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
