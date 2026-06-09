package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/MakerViking/TinkerView/internal/adapter"
	dockersrc "github.com/MakerViking/TinkerView/internal/adapters/docker"
	"github.com/MakerViking/TinkerView/internal/adapters/fake"
	"github.com/MakerViking/TinkerView/internal/adapters/frigate"
	go2rtcsrc "github.com/MakerViking/TinkerView/internal/adapters/go2rtc"
	"github.com/MakerViking/TinkerView/internal/automation"
	"github.com/MakerViking/TinkerView/internal/bus"
	"github.com/MakerViking/TinkerView/internal/camera"
	"github.com/MakerViking/TinkerView/internal/config"
	"github.com/MakerViking/TinkerView/internal/devices"
	"github.com/MakerViking/TinkerView/internal/hub"
	"github.com/MakerViking/TinkerView/internal/notify"
	"github.com/MakerViking/TinkerView/internal/rules"
	"github.com/MakerViking/TinkerView/internal/web"
)

// Version is set at build time via -ldflags "-X main.Version=...".
var Version = "dev"

// logFile holds the optional log file handle so main can close it on shutdown.
var logFile *os.File

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "/config.yaml", "path to config.yaml")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}
	setupLogging(cfg)
	if logFile != nil {
		defer logFile.Close()
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	b := bus.New()

	// --- supervised-goroutine launcher (WaitGroup + panic recovery) ---
	// Defined up front because the device manager launches adapter goroutines
	// through it (at startup and at runtime when devices are added from the UI).
	var wg sync.WaitGroup
	run := func(name string, fn func(context.Context)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					slog.Error("goroutine panic recovered", "name", name, "recover", r)
				}
			}()
			fn(ctx)
		}()
	}

	// --- writable device store (UI-managed printers/cameras + Frigate backups) ---
	// config.yaml is mounted read-only, so UI changes persist here under DataDir.
	probeDataDir(cfg.DataDir)
	store, err := devices.Open(filepath.Join(cfg.DataDir, "devices.yaml"))
	if err != nil {
		slog.Error("open device store", "err", err)
		os.Exit(1)
	}
	// First run: import the static config.yaml printers into the store so they
	// become editable/removable from the UI. Thereafter the store is authoritative.
	if store.Empty() && len(cfg.Printers) > 0 {
		for _, pc := range cfg.Printers {
			if err := store.UpsertPrinter(toRecord(pc)); err != nil {
				slog.Error("import static printer into store", "id", pc.ID, "err", err)
			}
		}
		slog.Info("imported static printers into device store", "count", len(cfg.Printers))
	}

	mgr := devices.NewManager(ctx, b, store, run)
	var extraRunners []func(context.Context)

	// Real camera enumeration: surface whatever go2rtc is configured to serve.
	if cfg.Go2RTCURL != "" {
		slog.Info("go2rtc camera source", "url", cfg.Go2RTCURL)
		extraRunners = append(extraRunners, go2rtcsrc.New(b, cfg.Go2RTCURL).Run)
	}

	// Frigate: online status, per-camera toggle states/commands, record ceilings, alerts.
	var frigateAdapter *frigate.Adapter
	if cfg.Frigate.MQTTAddr != "" {
		frigateAdapter = frigate.New(b, cfg.Frigate.MQTTAddr, cfg.Frigate.BaseURL, cfg.Frigate.TopicPrefix)
		extraRunners = append(extraRunners, frigateAdapter.Run)
		slog.Info("frigate adapter", "mqtt", cfg.Frigate.MQTTAddr, "http", cfg.Frigate.BaseURL)
	}

	// Notifications + automation rule engine.
	var notifier notify.Notifier = notify.Log{}
	if cfg.Ntfy.BaseURL != "" && cfg.Ntfy.Topic != "" {
		notifier = notify.NewNtfy(cfg.Ntfy.BaseURL, cfg.Ntfy.Topic, cfg.Ntfy.Token)
		slog.Info("ntfy notifier", "url", cfg.Ntfy.BaseURL, "topic", cfg.Ntfy.Topic)
	}
	// Notification rules: prioritize/suppress Frigate detections (known/unknown
	// face/plate + zone + time of day → push/silent/ignore). Seeded with sensible
	// defaults on first run; editable from the Recognition tab.
	ruleStore, err := rules.Open(filepath.Join(cfg.DataDir, "notify-rules.json"))
	if err != nil {
		slog.Error("open notification rules", "err", err)
		os.Exit(1)
	}
	extraRunners = append(extraRunners, automation.New(b, notifier, ruleStore).Run)

	// Server health (containers + disk) via a Docker-compatible API.
	var dockerSrc *dockersrc.Source
	if cfg.Docker.Enabled {
		ds, derr := dockersrc.New(b, cfg.Docker.Host, cfg.Docker.Watch, diskPaths(cfg))
		if derr != nil {
			slog.Error("docker health init failed; disabling", "err", derr)
		} else {
			dockerSrc = ds
			extraRunners = append(extraRunners, ds.Run)
			slog.Info("docker health source", "host", cfg.Docker.Host, "watch", cfg.Docker.Watch)
		}
	}

	// Camera manager: live-tile go2rtc streams + reconcile + Frigate promote/demote.
	g2client := go2rtcsrc.NewClient(cfg.Go2RTCURL)
	var frigateCfg *camera.FrigateConfig
	if cfg.Frigate.BaseURL != "" {
		frigateCfg = camera.NewFrigateConfig(cfg.Frigate.BaseURL, cfg.DataDir)
	}
	var frigateMQTT camera.FrigateTopicCleaner
	if frigateAdapter != nil {
		frigateMQTT = frigateAdapter // clears retained per-camera topics on remove/demote
	}
	cameraMgr := camera.NewManager(ctx, store, g2client, b, frigateCfg, frigateMQTT, cfg.HardwareTranscode)
	extraRunners = append(extraRunners, cameraMgr.Reconcile)

	cmd := &commandRouter{printers: mgr, cameras: cameraMgr, docker: dockerSrc, frigate: frigateAdapter}
	h := hub.New(b, cmd, cfg.Auth.Token)
	// cameraMgr edits Frigate's global config (known plates + detection hardware)
	// only when Frigate config editing is available; nil otherwise so the API
	// reports those endpoints as unavailable.
	var frigCfgMgr web.FrigateConfigManager
	if frigateCfg != nil {
		frigCfgMgr = cameraMgr
	}
	api := web.NewAPI(Version, h, cmd, notifier, frigCfgMgr, ruleStore)

	router, err := web.Router(cfg, h, api, mgr)
	if err != nil {
		slog.Error("build router", "err", err)
		os.Exit(1)
	}

	// --- start goroutines ---
	// Start the hub first so it captures adapter state as printers come up.
	run("hub", h.Run)
	for _, r := range store.Printers() {
		if err := mgr.RestorePrinter(r); err != nil { // launches the adapter via run()
			slog.Warn("restore printer", "id", r.ID, "err", err)
		}
	}
	// Demo mode adds a synthetic printer + two placeholder camera tiles so the UI
	// can be exercised with no real devices. Explicit opt-in via DASHBOARD_DEMO=1.
	if os.Getenv("DASHBOARD_DEMO") == "1" {
		slog.Info("demo mode: synthetic printer + placeholder cameras")
		mgr.AddRawAdapter(fake.NewPrinter(b, "demo", "Demo Printer"))
		extraRunners = append(extraRunners, fake.NewCameras(b, []string{"front_door", "printer1"}).Run)
	}
	for i, r := range extraRunners {
		r := r
		run(fmt.Sprintf("runner:%d", i), r)
	}

	// --- HTTP server ---
	srv := &http.Server{Addr: cfg.Listen, Handler: router, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(sctx); err != nil {
			slog.Warn("http server shutdown error", "err", err)
		}
	}()

	slog.Info("dashboard listening", "addr", cfg.Listen, "version", Version)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("http server", "err", err)
	}

	stop()    // ensure all goroutines see cancellation
	wg.Wait() // graceful drain
	slog.Info("shutdown complete")
}

// commandRouter implements hub.CommandHandler by delegating to the printer manager
// (lifecycle + commands + CRUD), the camera manager (live tiles + promote/demote),
// the Frigate adapter (per-camera toggles), and the Docker source (container
// restart). Named fields (not embedding) because both managers are type "Manager".
type commandRouter struct {
	printers *devices.Manager
	cameras  *camera.Manager
	docker   *dockersrc.Source
	frigate  *frigate.Adapter
}

func (c *commandRouter) PrinterCommand(ctx context.Context, id string, cmd adapter.PrinterCommand) error {
	return c.printers.PrinterCommand(ctx, id, cmd)
}

func (c *commandRouter) PrinterSetTemp(ctx context.Context, id, heater string, target float64) error {
	return c.printers.PrinterSetTemp(ctx, id, heater, target)
}

func (c *commandRouter) PrinterControl(ctx context.Context, id string, a adapter.PrinterAction) error {
	return c.printers.PrinterControl(ctx, id, a)
}

func (c *commandRouter) AddPrinter(r devices.PrinterRecord) error { return c.printers.AddPrinter(r) }
func (c *commandRouter) RemovePrinter(id string) error            { return c.printers.RemovePrinter(id) }
func (c *commandRouter) EditPrinter(id string, r devices.PrinterRecord) error {
	return c.printers.EditPrinter(id, r)
}

func (c *commandRouter) AddCamera(name, srcKind, srcInput, username, password string) error {
	return c.cameras.AddCamera(name, srcKind, srcInput, username, password)
}

func (c *commandRouter) EditCamera(name, newName, newSrcKind, newSrcInput, username, password string) error {
	return c.cameras.EditCamera(name, newName, newSrcKind, newSrcInput, username, password)
}

func (c *commandRouter) RemoveCamera(name string) error { return c.cameras.RemoveCamera(name) }

func (c *commandRouter) PromoteCamera(name string, detect, record bool, recordDays int, snapshots bool) error {
	return c.cameras.PromoteCamera(name, camera.PromoteOpts{
		Detect: detect, Record: record, RecordDays: recordDays, Snapshots: snapshots,
	})
}

func (c *commandRouter) DemoteCamera(name string) error { return c.cameras.DemoteCamera(name) }

func (c *commandRouter) EditCameraSettings(name string, s devices.CameraSettings) error {
	return c.cameras.EditCameraSettings(name, s)
}

func (c *commandRouter) CameraSettings(name string) (devices.CameraSettings, bool) {
	return c.cameras.Settings(name)
}

func (c *commandRouter) CameraToggle(ctx context.Context, cam string, feature adapter.CameraFeature, value, persist bool) error {
	if c.frigate == nil {
		return errors.New("Frigate is not configured")
	}
	return c.frigate.Toggle(ctx, cam, feature, value, persist)
}

func (c *commandRouter) ContainerRestart(ctx context.Context, id string) error {
	if c.docker == nil {
		return errors.New("server health (Docker) is disabled")
	}
	return c.docker.Restart(ctx, id)
}

// toRecord converts a static config.yaml printer into a device-store record (used
// for the one-time first-run import).
func toRecord(p config.Printer) devices.PrinterRecord {
	return devices.PrinterRecord{
		ID: p.ID, Kind: p.Kind, Name: p.Name, BaseURL: p.BaseURL, APIKey: p.APIKey,
		IP: p.IP, Serial: p.Serial, AccessCode: p.AccessCode, Model: p.Model,
		Camera: p.Camera, UIURL: p.UIURL, Macros: p.Macros,
	}
}

// probeDataDir ensures the writable data dir exists and is writable, logging a
// loud warning otherwise so CRUD-save failures are diagnosable at startup.
func probeDataDir(dir string) {
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		slog.Warn("data dir not creatable; adding/editing devices will fail", "dir", dir, "err", err)
		return
	}
	probe := filepath.Join(dir, ".write-probe")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		slog.Warn("data dir not writable; adding/editing devices will fail", "dir", dir, "err", err)
		return
	}
	_ = os.Remove(probe)
}

// diskPaths returns the filesystems to report usage for (Frigate media + root).
func diskPaths(cfg *config.Config) []string {
	seen := map[string]bool{}
	var paths []string
	for _, p := range []string{cfg.Frigate.MediaPath, "/"} {
		if p != "" && !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	return paths
}

// setupLogging configures the global slog logger from config: level (debug/info/
// warn/error) and an optional log file (appended to, in addition to stdout).
func setupLogging(cfg *config.Config) {
	var level slog.Level
	switch strings.ToLower(cfg.Log.Level) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	var w io.Writer = os.Stdout
	if cfg.Log.File != "" {
		f, err := os.OpenFile(cfg.Log.File, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			slog.Error("cannot open log file; logging to stdout only", "file", cfg.Log.File, "err", err)
		} else {
			logFile = f // closed in main via defer
			w = io.MultiWriter(os.Stdout, f)
		}
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})))
	slog.Info("logging configured", "level", level.String(), "file", cfg.Log.File)
}
