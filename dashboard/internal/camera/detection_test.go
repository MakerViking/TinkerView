package camera

import (
	"strings"
	"testing"

	"github.com/MakerViking/TinkerView/internal/devices"
)

const baseCfg = `mqtt:
  enabled: true
detectors:
  cpu1:
    type: cpu
model:
  path: /old/model.onnx
ffmpeg:
  hwaccel_args: preset-nvidia
cameras: {}
`

func TestDetectionRoundTripOpenVINO(t *testing.T) {
	raw, err := setDetectionYAML(baseCfg, devices.DetectionSettings{Detector: "openvino", Device: "GPU", Hwaccel: "vaapi"})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if !strings.Contains(raw, "enabled: true") {
		t.Fatalf("unrelated config lost:\n%s", raw)
	}
	got := parseDetection(raw)
	if got.Detector != "openvino" || got.Device != "GPU" {
		t.Fatalf("detector: %+v", got)
	}
	if got.Hwaccel != "vaapi" {
		t.Errorf("hwaccel should round-trip (preset- stripped): %q", got.Hwaccel)
	}
	if got.ModelPath != "/openvino-model/ssdlite_mobilenet_v2.xml" {
		t.Errorf("openvino bundled model not written: %q", got.ModelPath)
	}
	if got.Advanced {
		t.Error("known config should not be Advanced")
	}
}

func TestDetectionCPURemovesModelAndHwaccel(t *testing.T) {
	raw, err := setDetectionYAML(baseCfg, devices.DetectionSettings{Detector: "cpu"})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if strings.Contains(raw, "model:") {
		t.Errorf("model should be removed for cpu:\n%s", raw)
	}
	if strings.Contains(raw, "hwaccel_args") {
		t.Errorf("hwaccel should be removed when none selected:\n%s", raw)
	}
	got := parseDetection(raw)
	if got.Detector != "cpu" || got.Hwaccel != "" || got.ModelPath != "" {
		t.Fatalf("cpu parse: %+v", got)
	}
	if needsHostStep(got) {
		t.Error("cpu needs no host step")
	}
}

func TestDetectionOnnxRequiresModelPath(t *testing.T) {
	if _, err := setDetectionYAML(baseCfg, devices.DetectionSettings{Detector: "onnx"}); err == nil {
		t.Fatal("onnx without a model path should error")
	}
	raw, err := setDetectionYAML(baseCfg, devices.DetectionSettings{
		Detector: "onnx", ModelPath: "/config/model_cache/yolov9-t-320.onnx", ModelDtype: "float16", ModelWidth: 320, ModelHeight: 320,
	})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	got := parseDetection(raw)
	if got.Detector != "onnx" || got.ModelPath != "/config/model_cache/yolov9-t-320.onnx" || got.ModelDtype != "float16" {
		t.Fatalf("onnx round trip: %+v", got)
	}
	if !needsHostStep(got) {
		t.Error("onnx needs host steps (image + model)")
	}
}

func TestDetectionOpenVINOCPUNoHostStep(t *testing.T) {
	if needsHostStep(devices.DetectionSettings{Detector: "openvino", Device: "CPU"}) {
		t.Error("OpenVINO on CPU should need no host step")
	}
	if !needsHostStep(devices.DetectionSettings{Detector: "openvino", Device: "GPU"}) {
		t.Error("OpenVINO on GPU needs /dev/dri passthrough")
	}
}

func TestParseDetectionAdvanced(t *testing.T) {
	multi := `detectors:
  ov:
    type: openvino
    device: GPU
  coral:
    type: edgetpu
    device: pci
`
	if !parseDetection(multi).Advanced {
		t.Error("multiple detectors should be Advanced")
	}
	unknown := "detectors:\n  x:\n    type: somefuturedetector\n"
	if !parseDetection(unknown).Advanced {
		t.Error("unknown detector type should be Advanced")
	}
}

func TestBuildDetectorBlocksUnknown(t *testing.T) {
	if _, _, _, err := buildDetectorBlocks(devices.DetectionSettings{Detector: "bogus"}); err == nil {
		t.Fatal("unknown detector should error")
	}
}
