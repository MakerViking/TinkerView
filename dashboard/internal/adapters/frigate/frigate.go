// Package frigate integrates with Frigate over MQTT (+ a little HTTP): it tracks
// online status (frigate/available), per-camera toggle states
// (frigate/<cam>/<feature>/state), record ceilings (GET /api/config), and live
// alerts (frigate/reviews), and publishes per-camera toggle commands
// (frigate/<cam>/<feature>/set — Frigate has no HTTP endpoint for these).
package frigate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/MakerViking/TinkerView/internal/adapter"
	"github.com/MakerViking/TinkerView/internal/bus"
)

type Adapter struct {
	bus      *bus.Bus
	mqttAddr string
	httpBase string
	prefix   string
	http     *http.Client
	client   mqtt.Client

	mu  sync.Mutex
	seq int64
}

func New(b *bus.Bus, mqttAddr, httpBase, prefix string) *Adapter {
	if prefix == "" {
		prefix = "frigate"
	}
	return &Adapter{
		bus: b, mqttAddr: mqttAddr, httpBase: strings.TrimRight(httpBase, "/"), prefix: prefix,
		http: &http.Client{Timeout: 8 * time.Second},
	}
}

func (a *Adapter) Run(ctx context.Context) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(a.mqttAddr)
	opts.SetClientID("webcam-dashboard")
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(5 * time.Second)
	opts.SetOnConnectHandler(a.onConnect) // subscribe here so subs survive reconnects
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		slog.Warn("frigate: mqtt connection lost", "err", err)
		a.publishOnline(false)
	})
	cl := mqtt.NewClient(opts)
	a.mu.Lock()
	a.client = cl
	a.mu.Unlock()
	defer cl.Disconnect(200)

	if t := cl.Connect(); t.Wait() && t.Error() != nil {
		slog.Error("frigate: initial mqtt connect failed (will retry)", "err", t.Error())
	}

	// Periodically re-fetch /api/config so record ceilings recover from a slow
	// Frigate start and pick up mid-session config changes.
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.safeFetchConfig()
		}
	}
}

func (a *Adapter) getClient() mqtt.Client {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.client
}

func (a *Adapter) safeFetchConfig() {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("frigate: fetchConfig panic recovered", "recover", r)
		}
	}()
	a.fetchConfig()
}

func (a *Adapter) onConnect(c mqtt.Client) {
	slog.Info("frigate: mqtt connected", "broker", a.mqttAddr)
	c.Subscribe(a.prefix+"/available", 1, a.onAvailable)
	c.Subscribe(a.prefix+"/+/+/state", 1, a.onState)
	c.Subscribe(a.prefix+"/reviews", 1, a.onReview) // a dropped alert is unrecoverable
	go a.safeFetchConfig()                          // refresh camera list + record ceilings
}

func (a *Adapter) onAvailable(_ mqtt.Client, m mqtt.Message) {
	a.publishOnline(strings.EqualFold(strings.TrimSpace(string(m.Payload())), "online"))
}

func (a *Adapter) publishOnline(up bool) {
	a.bus.Publish(bus.Event{Source: bus.SourceFrigate, Kind: bus.KindFrigate, ID: "frigate", Ts: time.Now(), Payload: up})
}

// onState handles frigate/<cam>/<feature>/state.
func (a *Adapter) onState(_ mqtt.Client, m mqtt.Message) {
	parts := strings.Split(m.Topic(), "/")
	if len(parts) != 4 || parts[3] != "state" {
		return
	}
	cam := parts[1]
	payload := strings.TrimSpace(string(m.Payload()))
	if payload == "" {
		// A zero-length (cleared) retained message — e.g. a camera removed from
		// Frigate config whose retained topics we cleared. Don't resurrect a ghost
		// tile from it.
		return
	}
	on := strings.EqualFold(payload, "ON")
	cs := adapter.CameraState{Name: cam}
	switch parts[2] {
	case "detect":
		cs.Detect = &on
	case "recordings":
		cs.Recordings = &on
	case "snapshots":
		cs.Snapshots = &on
	case "motion":
		cs.Motion = &on
	case "enabled":
		cs.Enabled = &on
	default:
		return // ignore audio / improve_contrast / birdseye / etc.
	}
	a.bus.Publish(bus.Event{Source: bus.SourceFrigate, Kind: bus.KindCameraState, ID: cam, Ts: time.Now(), Payload: cs})
}

// onReview turns a new "alert" review segment into an in-app Alert.
func (a *Adapter) onReview(_ mqtt.Client, m mqtt.Message) {
	var rv struct {
		Type  string `json:"type"`
		After struct {
			ID       string `json:"id"`
			Camera   string `json:"camera"`
			Severity string `json:"severity"`
			Data     struct {
				Objects    []string `json:"objects"`
				Detections []string `json:"detections"`
			} `json:"data"`
		} `json:"after"`
	}
	if err := json.Unmarshal(m.Payload(), &rv); err != nil {
		slog.Warn("frigate: failed to parse review", "topic", m.Topic(), "err", err)
		return
	}
	if rv.Type != "new" || rv.After.Severity != "alert" {
		return
	}
	kind := "motion"
	for _, o := range rv.After.Data.Objects {
		if o == "person" {
			kind = "person"
			break
		}
	}
	a.mu.Lock()
	a.seq++
	seq := a.seq
	a.mu.Unlock()

	al := adapter.Alert{
		ID:      fmt.Sprintf("frigate-%s-%d", rv.After.ID, seq),
		Kind:    kind,
		Camera:  rv.After.Camera,
		Ts:      time.Now().UnixMilli(),
		Title:   capitalize(kind) + " — " + rv.After.Camera,
		Message: strings.Join(rv.After.Data.Objects, ", "),
	}
	if len(rv.After.Data.Detections) > 0 {
		ev := rv.After.Data.Detections[0]
		al.ThumbURL = "/frigate/api/events/" + ev + "/thumbnail.jpg"
		al.ClipURL = "/frigate/api/events/" + ev + "/clip.mp4"
	}
	a.bus.Publish(bus.Event{Source: bus.SourceFrigate, Kind: bus.KindAlert, ID: al.ID, Ts: time.Now(), Payload: al})
}

// fetchConfig enumerates Frigate cameras and their record ceiling (so the UI can
// disable the record toggle when recording isn't configured).
func (a *Adapter) fetchConfig() {
	resp, err := a.http.Get(a.httpBase + "/api/config")
	if err != nil {
		slog.Debug("frigate: /api/config unreachable", "err", err)
		return
	}
	defer resp.Body.Close()
	var cfg struct {
		Cameras map[string]struct {
			Record struct {
				Enabled bool `json:"enabled"`
			} `json:"record"`
		} `json:"cameras"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		slog.Debug("frigate: /api/config decode failed", "err", err)
		return
	}
	for name, c := range cfg.Cameras {
		rc := c.Record.Enabled
		a.bus.Publish(bus.Event{
			Source: bus.SourceFrigate, Kind: bus.KindCameraState, ID: name, Ts: time.Now(),
			Payload: adapter.CameraState{Name: name, RecordConfigured: &rc},
		})
	}
}

// Toggle publishes a per-camera feature command over MQTT (Frigate has no HTTP
// equivalent). persist is currently best-effort runtime only.
func (a *Adapter) Toggle(ctx context.Context, camera string, feature adapter.CameraFeature, value, _ bool) error {
	cl := a.getClient()
	if cl == nil || !cl.IsConnectionOpen() {
		return errors.New("Frigate MQTT not connected")
	}
	payload := "OFF"
	if value {
		payload = "ON"
	}
	topic := fmt.Sprintf("%s/%s/%s/set", a.prefix, camera, string(feature))
	tok := cl.Publish(topic, 0, false, payload)
	select {
	case <-tok.Done(): // preserve the publish ack
		return tok.Error()
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		return errors.New("Frigate: publish timed out")
	}
}

// ClearCameraTopics clears the retained per-camera state topics for a camera that
// has left Frigate (removed/demoted), so the dashboard won't re-discover a ghost
// tile from stale retained MQTT on the next reconnect. Best-effort.
func (a *Adapter) ClearCameraTopics(name string) {
	cl := a.getClient()
	if cl == nil || !cl.IsConnectionOpen() {
		return
	}
	for _, f := range []string{"detect", "recordings", "snapshots", "motion", "enabled", "audio", "improve_contrast"} {
		topic := fmt.Sprintf("%s/%s/%s/state", a.prefix, name, f)
		cl.Publish(topic, 0, true, []byte{}) // zero-length + retain clears the retained message
	}
	slog.Info("frigate: cleared retained state topics", "camera", name)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
