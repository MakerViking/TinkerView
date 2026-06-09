package camera

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/MakerViking/TinkerView/internal/devices"
)

// Frigate object-detection settings (the global detectors:/model:/ffmpeg: blocks).
// The dashboard can rewrite Frigate's CONFIG — but it cannot pass a GPU/Coral device
// into the Frigate container, swap the image variant (-tensorrt/-rocm), or export a
// model. Those host steps are surfaced in the UI per choice; here we only render and
// apply the config (mirroring the camera-settings flow: backup → validate → save +
// restart → health → rollback).

// DetectionSettings reads the current detector/model/hwaccel config from Frigate.
func (m *Manager) DetectionSettings(ctx context.Context) (devices.DetectionSettings, bool) {
	if m.frigate == nil {
		return devices.DetectionSettings{}, false
	}
	raw, err := m.frigate.GetRaw(ctx)
	if err != nil {
		slog.Warn("detection: read Frigate config failed", "err", err)
		return devices.DetectionSettings{}, false
	}
	return parseDetection(raw), true
}

// SetDetectionSettings validates the choice, then rewrites detectors:/model:/ffmpeg:
// and restarts Frigate asynchronously (reported via notices), like camera promote.
func (m *Manager) SetDetectionSettings(s devices.DetectionSettings) error {
	if m.frigate == nil {
		return fmt.Errorf("Frigate config editing is not available")
	}
	if _, _, _, err := buildDetectorBlocks(s); err != nil { // fail fast on a bad choice
		return err
	}
	m.async(func(ctx context.Context) {
		if err := m.applyDetection(ctx, s); err != nil {
			m.notice("error", err.Error())
		}
	})
	return nil
}

func (m *Manager) applyDetection(ctx context.Context, s devices.DetectionSettings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.frigate == nil {
		return fmt.Errorf("Frigate config editing is not available")
	}
	m.notice("info", "Applying detection settings — Frigate will restart (~25s)…")
	raw, err := m.frigate.GetRaw(ctx)
	if err != nil {
		return fmt.Errorf("read Frigate config: %w", err)
	}
	backupPath, err := m.frigate.backup(raw)
	if err != nil {
		return fmt.Errorf("back up Frigate config (aborting): %w", err)
	}
	newRaw, err := setDetectionYAML(raw, s)
	if err != nil {
		return fmt.Errorf("build Frigate config: %w", err)
	}
	if err := m.frigate.Validate(ctx, newRaw); err != nil {
		return fmt.Errorf("Frigate rejected the config: %w", err)
	}
	if err := m.frigate.SaveRestart(ctx, newRaw); err != nil {
		return fmt.Errorf("save Frigate config: %w", err)
	}
	if !m.frigate.WaitHealthy(ctx, frigateHealthTimeout) {
		m.restoreBackup(ctx, backupPath)
		return fmt.Errorf("Frigate did not come back after the detection change; restored previous config (backup: %s)", backupPath)
	}
	m.notice("info", m.detectorReport(ctx, s))
	slog.Info("detection settings applied", "detector", s.Detector, "device", s.Device, "hwaccel", s.Hwaccel)
	return nil
}

// detectorReport builds the success notice, polling /api/stats for a few seconds so
// it can report the new detector's actual inference speed (which only populates once
// the detector has processed frames — needs a detect-enabled camera sending video).
func (m *Manager) detectorReport(ctx context.Context, s devices.DetectionSettings) string {
	base := fmt.Sprintf("Detection set to %q", s.Detector)
	if s.Device != "" {
		base += " (" + s.Device + ")"
	}
	var best float64
	for i := 0; i < 6; i++ { // ~12s
		if speeds, err := m.frigate.DetectorSpeeds(ctx); err == nil {
			for _, ms := range speeds {
				if ms > best {
					best = ms
				}
			}
		}
		if best > 0 {
			break
		}
		select {
		case <-ctx.Done():
			return base
		case <-time.After(2 * time.Second):
		}
	}
	if best > 0 {
		return fmt.Sprintf("%s — running at %.1f ms/inference ✅", base, best)
	}
	if needsHostStep(s) {
		return base + " — loaded, but no inference yet. If it stays that way, finish the host prerequisites for this option (Detection tab)."
	}
	return base + " — loaded (inference speed appears once a detect-enabled camera sends frames)."
}

// needsHostStep reports whether the choice also requires a host action the dashboard
// can't perform (device passthrough / image variant / model export). CPU and
// OpenVINO-on-CPU are the only fully self-contained options.
func needsHostStep(s devices.DetectionSettings) bool {
	if s.Detector == "cpu" {
		return false
	}
	if s.Detector == "openvino" && strings.EqualFold(s.Device, "CPU") {
		return false
	}
	return true
}

// buildDetectorBlocks renders the detectors:/model: maps + the ffmpeg hwaccel string
// from known-good templates that match frigate/host-presets/*. model is nil when the
// detector uses a bundled model (cpu/edgetpu/hailo) — the caller deletes the key.
func buildDetectorBlocks(s devices.DetectionSettings) (detectors, model map[string]any, hwaccel string, err error) {
	switch s.Detector {
	case "cpu":
		detectors = map[string]any{"cpu": map[string]any{"type": "cpu"}}
	case "openvino":
		dev := s.Device
		if dev == "" {
			dev = "GPU"
		}
		detectors = map[string]any{"ov": map[string]any{"type": "openvino", "device": dev}}
		model = map[string]any{ // bundled OpenVINO model — no download
			"width": 300, "height": 300,
			"input_tensor": "nhwc", "input_pixel_format": "bgr",
			"path":          "/openvino-model/ssdlite_mobilenet_v2.xml",
			"labelmap_path": "/openvino-model/coco_91cl_bkgr.txt",
		}
	case "edgetpu":
		dev := s.Device
		if dev == "" {
			dev = "usb"
		}
		detectors = map[string]any{"coral": map[string]any{"type": "edgetpu", "device": dev}}
		// bundled EdgeTPU model — leave model nil
	case "onnx", "rocm":
		path := strings.TrimSpace(s.ModelPath)
		if path == "" {
			return nil, nil, "", fmt.Errorf("a model path is required for the %s detector — export the .onnx into frigate/model_cache first (see the Detection tab)", s.Detector)
		}
		mt := s.ModelType
		if mt == "" {
			if s.Detector == "onnx" {
				mt = "yolo-generic"
			} else {
				mt = "yolonas"
			}
		}
		w, h := s.ModelWidth, s.ModelHeight
		if w == 0 {
			w = 320
		}
		if h == 0 {
			h = 320
		}
		dtype := s.ModelDtype
		if dtype == "" {
			dtype = "float"
		}
		lbl := s.ModelLabelmap
		if lbl == "" {
			lbl = "/labelmap/coco-80.txt"
		}
		detectors = map[string]any{s.Detector: map[string]any{"type": s.Detector}}
		model = map[string]any{
			"model_type": mt, "width": w, "height": h,
			"input_tensor": "nchw", "input_dtype": dtype,
			"path": path, "labelmap_path": lbl,
		}
	case "hailo8l", "hailo8":
		detectors = map[string]any{"hailo": map[string]any{"type": s.Detector}}
		// bundled Hailo model — leave model nil
	default:
		return nil, nil, "", fmt.Errorf("unknown detector %q (want cpu|openvino|edgetpu|onnx|rocm|hailo8l)", s.Detector)
	}
	if hw := strings.TrimSpace(s.Hwaccel); hw != "" {
		hwaccel = "preset-" + hw
	}
	return detectors, model, hwaccel, nil
}

// setDetectionYAML replaces the top-level detectors:/model: keys and the
// ffmpeg.hwaccel_args scalar (comment/format-preserving), deleting model and
// hwaccel_args when the choice doesn't use them.
func setDetectionYAML(raw string, s devices.DetectionSettings) (string, error) {
	detectors, model, hwaccel, err := buildDetectorBlocks(s)
	if err != nil {
		return "", err
	}
	return editConfigYAML(raw, func(root *yaml.Node) error {
		dn, err := yamlValueNode(detectors)
		if err != nil {
			return err
		}
		setMapKey(root, "detectors", dn)

		if model != nil {
			mn, err := yamlValueNode(model)
			if err != nil {
				return err
			}
			setMapKey(root, "model", mn)
		} else {
			deleteMapKey(root, "model")
		}

		if hwaccel != "" {
			ff := ensureMap(root, "ffmpeg")
			setMapKey(ff, "hwaccel_args", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: hwaccel})
		} else if ff := getMapValue(root, "ffmpeg"); ff != nil {
			deleteMapKey(ff, "hwaccel_args")
		}
		return nil
	})
}

// parseDetection reads the current config back into the normalized struct so the UI
// can pre-fill. It maps the FIRST detector; configs it can't represent (multiple
// detectors, an unknown type, or a non-"preset-*" hwaccel) set Advanced so the UI
// warns before overwriting.
func parseDetection(raw string) devices.DetectionSettings {
	var doc struct {
		Detectors map[string]struct {
			Type   string `yaml:"type"`
			Device string `yaml:"device"`
		} `yaml:"detectors"`
		Model struct {
			ModelType  string `yaml:"model_type"`
			Width      int    `yaml:"width"`
			Height     int    `yaml:"height"`
			InputDtype string `yaml:"input_dtype"`
			Path       string `yaml:"path"`
			Labelmap   string `yaml:"labelmap_path"`
		} `yaml:"model"`
		Ffmpeg struct {
			Hwaccel stringOrSlice `yaml:"hwaccel_args"`
		} `yaml:"ffmpeg"`
	}
	out := devices.DetectionSettings{Detector: "cpu"}
	if err := yaml.Unmarshal([]byte(raw), &doc); err != nil {
		out.Advanced = true
		return out
	}

	known := map[string]bool{"cpu": true, "openvino": true, "edgetpu": true, "onnx": true, "rocm": true, "hailo8l": true, "hailo8": true}
	if len(doc.Detectors) > 1 {
		out.Advanced = true
	}
	for _, d := range doc.Detectors { // first (maps are unordered, but there's normally one)
		out.Detector = d.Type
		out.Device = d.Device
		if !known[d.Type] {
			out.Advanced = true
		}
		break
	}

	out.ModelType = doc.Model.ModelType
	out.ModelWidth = doc.Model.Width
	out.ModelHeight = doc.Model.Height
	out.ModelDtype = doc.Model.InputDtype
	out.ModelPath = doc.Model.Path
	out.ModelLabelmap = doc.Model.Labelmap

	if hw := []string(doc.Ffmpeg.Hwaccel); len(hw) == 1 {
		if v := strings.TrimPrefix(hw[0], "preset-"); v != hw[0] {
			out.Hwaccel = v
		} else if strings.TrimSpace(hw[0]) != "" {
			out.Advanced = true // a raw (non-preset) hwaccel string we can't round-trip
		}
	} else if len(hw) > 1 {
		out.Advanced = true
	}
	return out
}
