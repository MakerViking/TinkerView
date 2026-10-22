// Package camera manages UI-added cameras: it registers them as go2rtc streams
// for instant live tiles, reconciles those streams (go2rtc forgets runtime
// streams across a Frigate restart), and — when promoted — writes them into
// Frigate's config for NVR (see configclient + promote.go). The device store is
// the source of truth; this manager applies the go2rtc/Frigate side effects.
//
// Ops use the manager's own base context, never a client's request context, so a
// browser disconnect can't abort a Frigate restart mid-flight. Slow ops (promote/
// demote/remove-of-promoted) run asynchronously and report via notices.
package camera

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/MakerViking/TinkerView/internal/adapter"
	"github.com/MakerViking/TinkerView/internal/adapters/go2rtc"
	"github.com/MakerViking/TinkerView/internal/bus"
	"github.com/MakerViking/TinkerView/internal/devices"
)

const (
	// reconcileInterval re-asserts non-promoted store streams into go2rtc (offset
	// from the 15s enumeration poll in go2rtc.Source).
	reconcileInterval = 30 * time.Second
	// opTimeout bounds a fast op (go2rtc add/remove); slowTimeout covers a Frigate
	// save+restart+health-check.
	opTimeout   = 15 * time.Second
	slowTimeout = 90 * time.Second
)

// FrigateTopicCleaner clears a camera's retained Frigate MQTT state topics (so a
// removed/demoted camera doesn't leave a ghost tile). Implemented by the Frigate
// MQTT adapter; may be nil.
type FrigateTopicCleaner interface {
	ClearCameraTopics(name string)
}

// Manager owns the camera-side effects. All mutating CRUD ops are serialized by
// mu so a go2rtc/Frigate change never overlaps another.
type Manager struct {
	base        context.Context
	store       *devices.Store
	g2          *go2rtc.Client
	bus         *bus.Bus
	frigate     *FrigateConfig      // nil if Frigate config editing is unavailable
	frigateMQTT FrigateTopicCleaner // nil if Frigate MQTT is unavailable
	hardware    bool                // append go2rtc #hardware to transcoded sources

	mu sync.Mutex
}

func NewManager(base context.Context, store *devices.Store, g2 *go2rtc.Client, b *bus.Bus, frigate *FrigateConfig, frigateMQTT FrigateTopicCleaner, hardware bool) *Manager {
	return &Manager{base: base, store: store, g2: g2, bus: b, frigate: frigate, frigateMQTT: frigateMQTT, hardware: hardware}
}

// ----- public CRUD (hub.CommandHandler camera portion) -----

// AddCamera registers a new live-tile camera: validate, persist, push to go2rtc,
// and publish an instant tile. Synchronous (fast) so validation errors surface
// immediately.
func (m *Manager) AddCamera(name, srcKind, srcInput, username, password string) error {
	if err := devices.ValidateCameraName(name); err != nil {
		return err
	}
	src, err := m.resolveSrc(srcKind, srcInput)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(m.base, opTimeout)
	defer cancel()
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.store.Camera(name); exists {
		return fmt.Errorf("a camera named %q already exists", name)
	}
	if err := m.checkStreamCollision(ctx, name); err != nil {
		return err
	}
	rec := devices.CameraRecord{Name: name, Src: src, SrcKind: normKind(srcKind), Username: strings.TrimSpace(username), Password: password, Record: true}
	if err := m.store.UpsertCamera(rec); err != nil {
		return fmt.Errorf("save camera: %w", err)
	}
	if err := m.g2.PutStream(ctx, name, m.sourceWithTransform(rec)); err != nil {
		// Keep the record (reconcile will retry); report the failure.
		return fmt.Errorf("camera saved but go2rtc rejected the stream: %w", err)
	}
	m.publishManaged(rec)
	slog.Info("camera added", "name", name)
	return nil
}

// EditCamera is the positional form of Edit used by the WebSocket command: a
// blank username/password keeps the stored value (there is no way to clear
// credentials through it; the REST edit exposes ClearCredentials).
func (m *Manager) EditCamera(name, newName, newSrcKind, newSrcInput, username, password string) error {
	_, err := m.Edit(name, EditRequest{
		NewName: newName, NewSrcKind: newSrcKind, NewSrcInput: newSrcInput,
		Username: username, Password: password,
	})
	return err
}

// RemoveCamera deletes a camera. A live-tile camera is removed synchronously; a
// promoted camera requires a Frigate restart and is removed asynchronously (the
// outcome is reported via a notice).
func (m *Manager) RemoveCamera(name string) error {
	rec, ok := m.store.Camera(name)
	if !ok {
		return fmt.Errorf("unknown camera %q", name)
	}
	if rec.Promoted {
		m.async(func(ctx context.Context) {
			if err := m.removeCamera(ctx, name); err != nil {
				m.notice("error", err.Error())
			}
		})
		return nil
	}
	// Use the slow timeout even on the live path: if the camera was promoted
	// between the peek above and the locked re-check in removeCamera (a race with a
	// concurrent promote), removeCamera will need to restart Frigate and must not be
	// cut off by the short op timeout mid-restart.
	ctx, cancel := context.WithTimeout(m.base, slowTimeout)
	defer cancel()
	return m.removeCamera(ctx, name)
}

// removeCamera does the actual removal (handles the promoted case by removing
// from Frigate first). Caller provides a context with an appropriate timeout.
func (m *Manager) removeCamera(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.store.Camera(name)
	if !ok {
		return fmt.Errorf("unknown camera %q", name)
	}
	if rec.Promoted {
		// Remove from Frigate first; abort the whole removal if that fails so we
		// never leave Frigate half-edited.
		if err := m.removeFromFrigate(ctx, name); err != nil {
			return fmt.Errorf("remove from Frigate failed; camera kept: %w", err)
		}
	}
	// Also blank any printer's camera reference to it (one save), so a printer
	// panel doesn't keep embedding a dead stream.
	if _, _, err := m.store.DeleteCameraClearRefs(name); err != nil {
		return fmt.Errorf("save camera store: %w", err)
	}
	_ = m.g2.DeleteStream(ctx, name) // best-effort; reconcile won't re-add (gone from store)
	m.publishDeleted(name)
	slog.Info("camera removed", "name", name)
	return nil
}

// ----- reconcile loop -----

// Reconcile re-asserts non-promoted store streams into go2rtc (which forgets
// runtime streams across a Frigate restart) and republishes managed/promoted
// flags. Idempotent — only PUTs streams that are actually missing.
func (m *Manager) Reconcile(ctx context.Context) {
	t := time.NewTicker(reconcileInterval)
	defer t.Stop()
	m.reconcileOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.reconcileOnce(ctx)
		}
	}
}

func (m *Manager) reconcileOnce(ctx context.Context) {
	live, err := m.g2.ListStreams(ctx)
	for _, rec := range m.store.Cameras() {
		m.publishManaged(rec) // keep Managed/Promoted flags fresh for the UI
		if rec.Promoted || rec.ConfigStream {
			continue // Frigate's config owns this stream now
		}
		if err != nil {
			continue // go2rtc unreachable this tick; retry next
		}
		if _, ok := live[rec.Name]; !ok {
			// Re-check the store right before the PUT: a concurrent RemoveCamera may
			// have deleted it after we snapshotted Cameras(), and we must not re-add
			// a just-removed stream (it would linger in go2rtc until a restart).
			if _, still := m.store.Camera(rec.Name); !still {
				continue
			}
			if e := m.g2.PutStream(ctx, rec.Name, m.sourceWithTransform(rec)); e != nil {
				slog.Warn("camera reconcile: put failed", "name", rec.Name, "err", e)
			}
		}
	}
}

// ----- helpers -----

// sourceWithTransform returns the camera's go2rtc source with credentials and any
// rotate/flip transform applied. On a transform-build error (e.g. a source that
// already carries a raw ffmpeg filter) it logs and degrades to the un-transformed
// source so the stream keeps working; user-initiated saves validate the transform
// up front (see EditCameraSettings) so they surface the error instead.
func (m *Manager) sourceWithTransform(rec devices.CameraRecord) string {
	src, err := devices.BuildTransformSource(rec.Source(), rec.Rotate, rec.FlipH, rec.FlipV, m.hardware)
	if err != nil {
		slog.Warn("camera transform skipped", "name", rec.Name, "err", err)
		return rec.Source()
	}
	return src
}

// async runs fn on a recovering goroutine with a slow-op timeout derived from the
// manager's base context (so a client disconnect can't abort a Frigate restart).
func (m *Manager) async(fn func(context.Context)) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("camera op panic recovered", "recover", r)
			}
		}()
		ctx, cancel := context.WithTimeout(m.base, slowTimeout)
		defer cancel()
		fn(ctx)
	}()
}

// resolveSrc maps a UI source kind + URL to a go2rtc source string. Printer-cam
// MJPEG is transcoded to H.264 (needed for WebRTC); raw H.264 over HTTP likewise
// gets an ffmpeg wrapper; RTSP passes through. #hardware is added for transcodes
// only when the host has a GPU (cfg.HardwareTranscode).
func (m *Manager) resolveSrc(kind, input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("a source URL is required")
	}
	switch kind {
	case "rtsp":
		if !strings.HasPrefix(input, "rtsp://") && !strings.HasPrefix(input, "rtsps://") {
			return "", fmt.Errorf("RTSP source must start with rtsp://")
		}
		return input, checkUserSource(input)
	case "h264": // raw H.264 over HTTP (e.g. Snapmaker .h264)
		src := "ffmpeg:" + input + "#video=h264"
		return src, checkUserSource(src)
	case "mjpeg": // MJPEG printer cam → transcode to H.264 for WebRTC
		s := "ffmpeg:" + input + "#video=h264"
		if m.hardware {
			s += "#hardware"
		}
		return s, checkUserSource(s)
	case "mjpeg_raw": // cheap MJPEG passthrough (MSE/MJPEG fallback only)
		return input, checkUserSource(input)
	case "raw", "advanced", "":
		return input, checkUserSource(input) // power-user go2rtc source, minus process-spawning schemes
	default:
		return "", fmt.Errorf("unknown source kind %q", kind)
	}
}

// checkStreamCollision rejects a name that already exists as a go2rtc stream the
// dashboard doesn't own (a Frigate-managed / pre-existing camera, or its _hq
// variant). Best-effort: if go2rtc is unreachable, allow the add and let
// reconcile handle it. Caller holds m.mu.
func (m *Manager) checkStreamCollision(ctx context.Context, name string) error {
	live, err := m.g2.ListStreams(ctx)
	if err != nil {
		slog.Warn("camera collision check skipped (go2rtc unreachable)", "err", err)
		return nil
	}
	if _, ok := live[name]; ok {
		if _, mine := m.store.Camera(name); !mine {
			return fmt.Errorf("a stream named %q already exists in go2rtc/Frigate", name)
		}
	}
	if _, ok := live[name+"_hq"]; ok {
		return fmt.Errorf("a stream named %q already exists", name+"_hq")
	}
	return nil
}

func (m *Manager) publishManaged(rec devices.CameraRecord) {
	// Adopted hand-authored cameras (ConfigStream) are only image-transform managed —
	// they stay read-only in the UI (no edit/remove/promote), so report managed:false.
	managed := !rec.ConfigStream
	promoted := rec.Promoted
	m.bus.Publish(bus.Event{
		Source: bus.SourceFrigate, Kind: bus.KindCameraState, ID: rec.Name, Ts: time.Now(),
		Payload: adapter.CameraState{Name: rec.Name, Managed: &managed, Promoted: &promoted},
	})
}

func (m *Manager) publishDeleted(name string) {
	m.bus.Publish(bus.Event{
		Source: bus.SourceFrigate, Kind: bus.KindCameraState, ID: name, Ts: time.Now(),
		Payload: adapter.CameraState{Name: name, Deleted: true},
	})
}

func (m *Manager) notice(level, msg string) {
	m.bus.Publish(bus.Event{
		Source: bus.SourceSystem, Kind: bus.KindNotice, Ts: time.Now(),
		Payload: adapter.Notice{Level: level, Msg: msg},
	})
}
