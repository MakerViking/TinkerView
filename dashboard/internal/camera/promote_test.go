package camera

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// sampleConfig mirrors the hand-authored frigate/config.yml shape: comments, a
// host-specific block, {ENV} substitutions, and go2rtc.streams + cameras maps.
const sampleConfig = `# Frigate config — hand authored, keep comments!
detectors:
  cpu1:
    type: cpu # TEST-ONLY

mqtt:
  enabled: true
  host: 127.0.0.1

# --- go2rtc restream/fan-out ---
go2rtc:
  streams:
    # security cam
    front_door:
      - rtsp://{FRIGATE_RTSP_USER}:{FRIGATE_RTSP_PASSWORD}@192.168.1.50:554/Streaming/Channels/101
    printer1:
      - ffmpeg:http://192.168.1.80/webcam/?action=stream#video=h264#hardware

record:
  enabled: true

# --- Cameras ---
cameras:
  printer1:
    enabled: true
    detect:
      enabled: false
    live:
      stream_name: printer1
`

// TestRoundTripPreservesComments: a no-op edit keeps comments and {ENV}
// placeholders — a guard against a yaml.v3 change silently corrupting a live config.
func TestRoundTripPreservesComments(t *testing.T) {
	out, err := editConfigYAML(sampleConfig, func(*yaml.Node) error { return nil })
	if err != nil {
		t.Fatalf("round-trip: %v", err)
	}
	for _, want := range []string{
		"# Frigate config — hand authored, keep comments!",
		"# --- go2rtc restream/fan-out ---",
		"# --- Cameras ---",
		"# security cam",
		"{FRIGATE_RTSP_PASSWORD}",
		"#video=h264#hardware",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("round-trip dropped %q\n--- output ---\n%s", want, out)
		}
	}
}

func TestInsertAndRemoveCamera(t *testing.T) {
	withCam, err := insertCameraYAML(sampleConfig, "garage", "rtsp://10.0.0.9/live", PromoteOpts{Record: true, RecordDays: 3})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	// Must still parse and contain both the new stream and camera block.
	var parsed struct {
		Go2RTC struct {
			Streams map[string]any `yaml:"streams"`
		} `yaml:"go2rtc"`
		Cameras map[string]any `yaml:"cameras"`
	}
	if err := yaml.Unmarshal([]byte(withCam), &parsed); err != nil {
		t.Fatalf("inserted config does not parse: %v\n%s", err, withCam)
	}
	if _, ok := parsed.Go2RTC.Streams["garage"]; !ok {
		t.Errorf("garage stream not inserted")
	}
	if _, ok := parsed.Cameras["garage"]; !ok {
		t.Errorf("garage camera block not inserted")
	}
	if _, ok := parsed.Cameras["printer1"]; !ok {
		t.Errorf("existing printer1 camera lost after insert")
	}
	// Restream path convention.
	if !strings.Contains(withCam, "rtsp://127.0.0.1:8554/garage") {
		t.Errorf("camera input should reference the go2rtc restream; got:\n%s", withCam)
	}

	// Removing it returns to a config without garage, keeping printer1 + comments.
	back, err := removeCameraYAML(withCam, "garage")
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if strings.Contains(back, "garage") {
		t.Errorf("garage not fully removed:\n%s", back)
	}
	for _, want := range []string{"printer1", "# --- Cameras ---", "{FRIGATE_RTSP_PASSWORD}"} {
		if !strings.Contains(back, want) {
			t.Errorf("remove dropped %q", want)
		}
	}
}
