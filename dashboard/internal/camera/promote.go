package camera

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/MakerViking/TinkerView/internal/devices"
)

// frigateHealthTimeout bounds how long we wait for Frigate to come back after a
// config save+restart before rolling back.
const frigateHealthTimeout = 45 * time.Second

// PromoteOpts are the Frigate NVR options chosen when promoting a camera.
type PromoteOpts struct {
	Detect     bool
	Record     bool
	RecordDays int
	Snapshots  bool
}

// PromoteCamera validates quickly, then runs the (slow, Frigate-restarting)
// promotion asynchronously so it isn't tied to the client connection. Progress
// and the final outcome are reported via notices.
func (m *Manager) PromoteCamera(name string, opts PromoteOpts) error {
	if m.frigate == nil {
		return fmt.Errorf("Frigate config editing is not available")
	}
	if _, ok := m.store.Camera(name); !ok {
		return fmt.Errorf("unknown camera %q", name)
	}
	m.async(func(ctx context.Context) {
		if err := m.promoteWork(ctx, name, opts); err != nil {
			m.notice("error", err.Error())
		}
	})
	return nil
}

// promoteWork writes a managed live-tile camera into Frigate's config (go2rtc
// stream + camera block) and restarts Frigate. The sequence is: backup → build →
// validate(saveonly) → persist intent → save(restart) → health-check → rollback
// on failure. After promotion the reconcile loop leaves the stream to Frigate.
func (m *Manager) promoteWork(ctx context.Context, name string, opts PromoteOpts) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.frigate == nil {
		return fmt.Errorf("Frigate config editing is not available")
	}
	rec, ok := m.store.Camera(name)
	if !ok {
		return fmt.Errorf("unknown camera %q", name)
	}
	if rec.Promoted {
		return fmt.Errorf("camera %q is already promoted", name)
	}
	// Apply the chosen NVR options to the record up front so the camera block is
	// built from the same record we persist.
	rec.Promoted = true
	rec.Detect, rec.Record, rec.RecordDays, rec.Snapshots = opts.Detect, opts.Record, opts.RecordDays, opts.Snapshots

	m.notice("info", fmt.Sprintf("Promoting %q to Frigate — Frigate will restart (~25s)…", name))
	raw, err := m.frigate.GetRaw(ctx)
	if err != nil {
		return fmt.Errorf("read Frigate config: %w", err)
	}
	backupPath, err := m.frigate.backup(raw)
	if err != nil {
		return fmt.Errorf("back up Frigate config (aborting): %w", err)
	}
	newRaw, err := insertCameraYAML(raw, name, m.sourceWithTransform(rec), rec)
	if err != nil {
		return fmt.Errorf("build Frigate config: %w", err)
	}
	if err := m.frigate.Validate(ctx, newRaw); err != nil {
		return fmt.Errorf("Frigate rejected the config: %w", err) // no restart happened
	}

	// Persist intent before the restart so a crash heals to the promoted state.
	if err := m.store.UpsertCamera(rec); err != nil {
		return fmt.Errorf("save camera: %w", err)
	}
	if err := m.frigate.SaveRestart(ctx, newRaw); err != nil {
		m.revertPromoteIntent(rec)
		return fmt.Errorf("save Frigate config: %w", err)
	}
	if !m.frigate.WaitHealthy(ctx, frigateHealthTimeout) {
		m.restoreBackup(ctx, backupPath)
		m.revertPromoteIntent(rec)
		return fmt.Errorf("Frigate did not come back after promote; restored previous config (backup: %s)", backupPath)
	}
	m.publishManaged(rec)
	m.notice("info", fmt.Sprintf("Camera %q promoted to Frigate NVR", name))
	slog.Info("camera promoted", "name", name)
	return nil
}

// editPromotedSource re-applies a promoted camera's updated record (new source/
// credentials) to Frigate: it rewrites only the go2rtc stream entry and restarts
// Frigate, leaving the camera block and NVR options untouched. The store is
// updated before the restart (so a crash heals to the new state) and reverted if
// Frigate fails to come back. `want` carries the desired Src/Username/Password.
func (m *Manager) editPromotedSource(ctx context.Context, want devices.CameraRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.frigate == nil {
		return fmt.Errorf("Frigate config editing is not available")
	}
	cur, ok := m.store.Camera(want.Name)
	if !ok {
		return fmt.Errorf("unknown camera %q", want.Name)
	}
	if !cur.Promoted {
		return fmt.Errorf("camera %q is no longer promoted", want.Name)
	}
	m.notice("info", fmt.Sprintf("Updating %q — Frigate will restart (~25s)…", want.Name))
	raw, err := m.frigate.GetRaw(ctx)
	if err != nil {
		return fmt.Errorf("read Frigate config: %w", err)
	}
	backupPath, err := m.frigate.backup(raw)
	if err != nil {
		return fmt.Errorf("back up Frigate config (aborting): %w", err)
	}
	// Carry the new source/credentials onto the current record (preserve NVR options).
	next := cur
	next.Src, next.Username, next.Password = want.Src, want.Username, want.Password
	newRaw, err := updateStreamSourceYAML(raw, next.Name, m.sourceWithTransform(next))
	if err != nil {
		return fmt.Errorf("build Frigate config: %w", err)
	}
	if err := m.frigate.Validate(ctx, newRaw); err != nil {
		return fmt.Errorf("Frigate rejected the config: %w", err)
	}
	if err := m.store.UpsertCamera(next); err != nil {
		return fmt.Errorf("save camera: %w", err)
	}
	if err := m.frigate.SaveRestart(ctx, newRaw); err != nil {
		_ = m.store.UpsertCamera(cur) // revert store to match the un-saved Frigate config
		return fmt.Errorf("save Frigate config: %w", err)
	}
	if !m.frigate.WaitHealthy(ctx, frigateHealthTimeout) {
		m.restoreBackup(ctx, backupPath)
		_ = m.store.UpsertCamera(cur)
		return fmt.Errorf("Frigate did not come back after the edit; restored previous config (backup: %s)", backupPath)
	}
	m.publishManaged(next)
	m.notice("info", fmt.Sprintf("Camera %q updated", next.Name))
	slog.Info("promoted camera source edited", "name", next.Name)
	return nil
}

// Settings returns a camera's current Frigate-NVR settings from the store (used to
// pre-fill the settings UI so a save doesn't clobber existing zones).
func (m *Manager) Settings(name string) (devices.CameraSettings, bool) {
	rec, ok := m.store.Camera(name)
	if !ok {
		return devices.CameraSettings{}, false
	}
	return devices.CameraSettings{
		Detect:     rec.Detect,
		Record:     rec.Record,
		RecordMode: rec.RecordMode,
		RecordDays: rec.RecordDays,
		Snapshots:  rec.Snapshots,
		Objects:    rec.Objects,
		Zones:      rec.Zones,
		Rotate:     rec.Rotate,
		FlipH:      rec.FlipH,
		FlipV:      rec.FlipV,
	}, true
}

// EditCameraSettings updates a camera's Frigate-NVR settings (detect, record mode,
// snapshots, tracked objects, detection zones). For a non-promoted camera the
// settings are just persisted and take effect when it is next promoted. For a
// promoted camera the cameras[<name>] block is rewritten and Frigate restarted
// asynchronously (mirroring editPromotedSource), reported via notices.
func (m *Manager) EditCameraSettings(name string, s devices.CameraSettings) error {
	m.mu.Lock()
	rec, inStore := m.store.Camera(name)
	m.mu.Unlock()

	// Not in the store → a hand-authored Frigate-config camera. Adopt it and apply
	// the image transform ONLY (we never touch its hand-authored detect/record/zones),
	// asynchronously since it edits the Frigate config (and may restart Frigate).
	if !inStore {
		m.async(func(ctx context.Context) {
			if err := m.adoptAndApplyTransform(ctx, name, s); err != nil {
				m.notice("error", err.Error())
			}
		})
		return nil
	}

	// Validate the requested transform up front so an unsupported source (exec:, or
	// one that already carries a raw ffmpeg filter) surfaces the error to the user
	// instead of silently no-op'ing.
	if _, err := devices.BuildTransformSource(rec.Source(), s.Rotate, s.FlipH, s.FlipV, m.hardware); err != nil {
		return err
	}

	// Adopted config-stream camera: only the image transform is dashboard-managed;
	// rewrite the Frigate go2rtc stream + restart (async).
	if rec.ConfigStream {
		m.async(func(ctx context.Context) {
			if err := m.applyCameraSettings(ctx, name, s); err != nil {
				m.notice("error", err.Error())
			}
		})
		return nil
	}

	if !rec.Promoted {
		// UI-added live tile: persist the settings (detect/record take effect on the
		// next promote) and re-PUT the runtime go2rtc stream so the transform applies
		// to the live tile immediately.
		m.mu.Lock()
		applySettings(&rec, s)
		err := m.store.UpsertCamera(rec)
		if err == nil {
			m.publishManaged(rec)
		}
		src := m.sourceWithTransform(rec)
		m.mu.Unlock()
		if err != nil {
			return fmt.Errorf("save camera: %w", err)
		}
		ctx, cancel := context.WithTimeout(m.base, opTimeout)
		defer cancel()
		if err := m.g2.PutStream(ctx, name, src); err != nil {
			return fmt.Errorf("update live stream: %w", err)
		}
		return nil
	}

	// Managed + promoted: rewrite the Frigate config (stream + camera block) + restart.
	m.async(func(ctx context.Context) {
		if err := m.applyCameraSettings(ctx, name, s); err != nil {
			m.notice("error", err.Error())
		}
	})
	return nil
}

// applyCameraSettings applies new settings to a stored Frigate-config camera (a
// managed+promoted camera, or an adopted hand-authored one) and restarts Frigate.
// A managed+promoted camera's whole cameras[<name>] block is rebuilt from the
// settings; an adopted (ConfigStream) camera only gets its go2rtc stream rewritten
// for the image transform — its hand-authored block is left untouched.
func (m *Manager) applyCameraSettings(ctx context.Context, name string, s devices.CameraSettings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.frigate == nil {
		return fmt.Errorf("Frigate config editing is not available")
	}
	cur, ok := m.store.Camera(name)
	if !ok {
		return fmt.Errorf("unknown camera %q", name)
	}
	if !cur.Promoted && !cur.ConfigStream {
		return fmt.Errorf("camera %q is not a Frigate-config camera", name)
	}
	next := cur
	if cur.ConfigStream {
		applyTransformOnly(&next, s) // hand-authored cam: image transform only
	} else {
		applySettings(&next, s) // dashboard owns the full camera block
	}
	m.notice("info", fmt.Sprintf("Applying %q settings — Frigate will restart (~25s)…", name))
	return m.applyConfigStreamLocked(ctx, next, func() { _ = m.store.UpsertCamera(cur) })
}

// adoptAndApplyTransform brings a hand-authored Frigate-config camera under the
// dashboard's image-transform management: it reads the camera's base go2rtc source
// from the live config, persists a ConfigStream record carrying only the requested
// transform, then rewrites the go2rtc stream + restarts Frigate. The camera stays
// otherwise read-only in the UI (managed:false). Caller runs this asynchronously.
func (m *Manager) adoptAndApplyTransform(ctx context.Context, name string, s devices.CameraSettings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.frigate == nil {
		return fmt.Errorf("Frigate config editing is not available")
	}
	if cur, exists := m.store.Camera(name); exists {
		// Raced with another adopt; apply onto the now-stored record.
		next := cur
		applyTransformOnly(&next, s)
		return m.applyConfigStreamLocked(ctx, next, func() { _ = m.store.UpsertCamera(cur) })
	}
	raw, err := m.frigate.GetRaw(ctx)
	if err != nil {
		return fmt.Errorf("read Frigate config: %w", err)
	}
	base, isCamera, err := readConfigStream(raw, name)
	if err != nil {
		return err
	}
	rec := devices.CameraRecord{Name: name, Src: base, ConfigStream: true, Promoted: isCamera}
	applyTransformOnly(&rec, s)
	if _, err := devices.BuildTransformSource(rec.Source(), rec.Rotate, rec.FlipH, rec.FlipV, m.hardware); err != nil {
		return err
	}
	m.notice("info", fmt.Sprintf("Applying image settings to %q — Frigate will restart (~25s)…", name))
	return m.applyConfigStreamLocked(ctx, rec, func() {
		_, _ = m.store.DeleteCamera(name)
		m.publishDeleted(name)
	})
}

// applyConfigStreamLocked rewrites a Frigate-config camera's go2rtc stream (and, for
// a managed+promoted camera, its cameras[<name>] block) from `next`, then validates,
// persists, saves+restarts Frigate, and health-checks — calling revert (store
// rollback) if the save or restart fails. Caller holds m.mu.
func (m *Manager) applyConfigStreamLocked(ctx context.Context, next devices.CameraRecord, revert func()) error {
	name := next.Name
	raw, err := m.frigate.GetRaw(ctx)
	if err != nil {
		return fmt.Errorf("read Frigate config: %w", err)
	}
	backupPath, err := m.frigate.backup(raw)
	if err != nil {
		return fmt.Errorf("back up Frigate config (aborting): %w", err)
	}
	src := m.sourceWithTransform(next)
	// Rebuild the camera block only when the dashboard fully owns it (managed+promoted);
	// for adopted (ConfigStream) cameras we touch only the go2rtc stream.
	if next.Promoted && !next.ConfigStream {
		raw, err = updateCameraStreamAndBlockYAML(raw, name, src, next)
	} else {
		raw, err = updateStreamSourceYAML(raw, name, src)
	}
	if err != nil {
		return fmt.Errorf("build Frigate config: %w", err)
	}
	if err := m.frigate.Validate(ctx, raw); err != nil {
		return fmt.Errorf("Frigate rejected the config: %w", err)
	}
	if err := m.store.UpsertCamera(next); err != nil {
		return fmt.Errorf("save camera: %w", err)
	}
	if err := m.frigate.SaveRestart(ctx, raw); err != nil {
		revert()
		return fmt.Errorf("save Frigate config: %w", err)
	}
	if !m.frigate.WaitHealthy(ctx, frigateHealthTimeout) {
		m.restoreBackup(ctx, backupPath)
		revert()
		return fmt.Errorf("Frigate did not come back after the settings change; restored previous config (backup: %s)", backupPath)
	}
	m.publishManaged(next)
	m.notice("info", fmt.Sprintf("Camera %q settings applied", name))
	slog.Info("camera settings applied", "name", name)
	return nil
}

// applySettings copies CameraSettings onto a record (in place).
func applySettings(r *devices.CameraRecord, s devices.CameraSettings) {
	r.Detect = s.Detect
	r.Record = s.Record
	r.RecordMode = s.RecordMode
	r.RecordDays = s.RecordDays
	r.Snapshots = s.Snapshots
	r.Objects = s.Objects
	r.Zones = s.Zones
	r.Rotate = s.Rotate
	r.FlipH = s.FlipH
	r.FlipV = s.FlipV
}

// applyTransformOnly copies just the image-transform fields onto a record — used for
// adopted hand-authored cameras, whose detect/record/zones config the dashboard does
// not own and must not overwrite.
func applyTransformOnly(r *devices.CameraRecord, s devices.CameraSettings) {
	r.Rotate = s.Rotate
	r.FlipH = s.FlipH
	r.FlipV = s.FlipV
}

// DemoteCamera validates quickly, then runs the (slow, Frigate-restarting)
// demotion asynchronously; the outcome is reported via notices.
func (m *Manager) DemoteCamera(name string) error {
	if m.frigate == nil {
		return fmt.Errorf("Frigate config editing is not available")
	}
	if _, ok := m.store.Camera(name); !ok {
		return fmt.Errorf("unknown camera %q", name)
	}
	m.async(func(ctx context.Context) {
		if err := m.demoteWork(ctx, name); err != nil {
			m.notice("error", err.Error())
		}
	})
	return nil
}

// demoteWork removes a camera from Frigate's config (restart) but keeps it as a
// live go2rtc tile.
func (m *Manager) demoteWork(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.store.Camera(name)
	if !ok {
		return fmt.Errorf("unknown camera %q", name)
	}
	if !rec.Promoted {
		return fmt.Errorf("camera %q is not promoted", name)
	}
	if err := m.removeFromFrigate(ctx, name); err != nil {
		return err
	}
	rec.Promoted = false
	if err := m.store.UpsertCamera(rec); err != nil {
		return fmt.Errorf("save camera: %w", err)
	}
	_ = m.g2.PutStream(ctx, name, m.sourceWithTransform(rec)) // resume the live tile immediately
	m.publishManaged(rec)
	m.notice("info", fmt.Sprintf("Camera %q demoted; kept as a live tile", name))
	slog.Info("camera demoted", "name", name)
	return nil
}

// removeFromFrigate deletes the camera + its go2rtc stream from Frigate's config
// and restarts, with backup + validate + health-check + rollback. It does NOT
// touch the device store (callers do, only on success).
func (m *Manager) removeFromFrigate(ctx context.Context, name string) error {
	if m.frigate == nil {
		return fmt.Errorf("Frigate config editing is not available")
	}
	m.notice("info", fmt.Sprintf("Updating Frigate for %q — Frigate will restart (~25s)…", name))
	raw, err := m.frigate.GetRaw(ctx)
	if err != nil {
		return fmt.Errorf("read Frigate config: %w", err)
	}
	backupPath, err := m.frigate.backup(raw)
	if err != nil {
		return fmt.Errorf("back up Frigate config (aborting): %w", err)
	}
	newRaw, err := removeCameraYAML(raw, name)
	if err != nil {
		return fmt.Errorf("edit Frigate config: %w", err)
	}
	if err := m.frigate.Validate(ctx, newRaw); err != nil {
		return fmt.Errorf("Frigate rejected the config: %w", err)
	}
	if err := m.frigate.SaveRestart(ctx, newRaw); err != nil {
		return fmt.Errorf("save Frigate config: %w", err)
	}
	if !m.frigate.WaitHealthy(ctx, frigateHealthTimeout) {
		m.restoreBackup(ctx, backupPath)
		return fmt.Errorf("Frigate did not come back; restored previous config (backup: %s)", backupPath)
	}
	// Clear the camera's retained Frigate MQTT state topics so it doesn't leave a
	// ghost tile that the dashboard re-discovers on reconnect.
	if m.frigateMQTT != nil {
		m.frigateMQTT.ClearCameraTopics(name)
	}
	return nil
}

func (m *Manager) revertPromoteIntent(rec devices.CameraRecord) {
	rec.Promoted = false
	if err := m.store.UpsertCamera(rec); err != nil {
		slog.Error("failed to revert promote intent in store", "name", rec.Name, "err", err)
		// Loud: the store now disagrees with Frigate (store thinks promoted, Frigate
		// rolled back) — surface it so the operator can fix it manually.
		m.notice("error", fmt.Sprintf("camera %q: promote failed AND reverting the saved state failed (%v) — check the device store", rec.Name, err))
	}
}

// restoreBackup re-saves a backed-up raw config (best-effort) when a restart
// fails to bring Frigate back.
func (m *Manager) restoreBackup(ctx context.Context, path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		m.notice("error", fmt.Sprintf("could not read backup %s to restore Frigate: %v", path, err))
		return
	}
	if err := m.frigate.SaveRestart(ctx, string(raw)); err != nil {
		m.notice("error", fmt.Sprintf("FAILED to restore Frigate from backup %s — fix the config manually: %v", path, err))
	}
}

// ----- yaml.Node editing (comment/format-preserving) -----

// insertCameraYAML inserts go2rtc.streams[name]=[src] and a cameras[name] block.
func insertCameraYAML(raw, name, src string, rec devices.CameraRecord) (string, error) {
	return editConfigYAML(raw, func(root *yaml.Node) error {
		streams := ensureMap(ensureMap(root, "go2rtc"), "streams")
		srcNode, err := yamlValueNode([]string{src})
		if err != nil {
			return err
		}
		setMapKey(streams, name, srcNode)

		cams := ensureMap(root, "cameras")
		block, err := yamlValueNode(cameraBlock(name, rec))
		if err != nil {
			return err
		}
		setMapKey(cams, name, block)
		return nil
	})
}

// updateCameraStreamAndBlockYAML rewrites both go2rtc.streams[name] (= [src], for
// the image transform) and the cameras[name] block (freshly built from the record)
// in a single edit — used when a managed+promoted camera's settings change.
func updateCameraStreamAndBlockYAML(raw, name, src string, rec devices.CameraRecord) (string, error) {
	return editConfigYAML(raw, func(root *yaml.Node) error {
		streams := ensureMap(ensureMap(root, "go2rtc"), "streams")
		srcNode, err := yamlValueNode([]string{src})
		if err != nil {
			return err
		}
		setMapKey(streams, name, srcNode)

		cams := ensureMap(root, "cameras")
		block, err := yamlValueNode(cameraBlock(name, rec))
		if err != nil {
			return err
		}
		setMapKey(cams, name, block)
		return nil
	})
}

// readConfigStream reads a hand-authored camera's base go2rtc source from the raw
// Frigate config (go2rtc.streams[name]) and reports whether it is also a Frigate NVR
// camera (cameras[name] exists). It errors when the camera has no go2rtc restream
// (image controls don't apply) or has multiple sources (unsupported).
func readConfigStream(raw, name string) (base string, isCamera bool, err error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &doc); err != nil {
		return "", false, fmt.Errorf("parse config: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return "", false, fmt.Errorf("unexpected config shape")
	}
	root := doc.Content[0]
	noStream := fmt.Errorf("camera %q isn't restreamed through go2rtc, so image controls aren't available — re-add it through the dashboard", name)
	g2 := getMapValue(root, "go2rtc")
	if g2 == nil {
		return "", false, noStream
	}
	streams := getMapValue(g2, "streams")
	if streams == nil {
		return "", false, noStream
	}
	sv := getMapValue(streams, name)
	if sv == nil {
		return "", false, noStream
	}
	switch sv.Kind {
	case yaml.ScalarNode:
		base = sv.Value
	case yaml.SequenceNode:
		if len(sv.Content) == 0 {
			return "", false, fmt.Errorf("camera %q has an empty go2rtc stream", name)
		}
		if len(sv.Content) > 1 {
			return "", false, fmt.Errorf("camera %q has multiple go2rtc sources; image controls aren't supported for it", name)
		}
		base = sv.Content[0].Value
	default:
		return "", false, fmt.Errorf("camera %q has an unexpected go2rtc stream format", name)
	}
	if base == "" {
		return "", false, fmt.Errorf("camera %q has an empty go2rtc source", name)
	}
	if cams := getMapValue(root, "cameras"); cams != nil && getMapValue(cams, name) != nil {
		isCamera = true
	}
	return base, isCamera, nil
}

// updateStreamSourceYAML rewrites go2rtc.streams[name] = [src] without touching
// the cameras[name] block — used to change a promoted camera's source/credentials.
func updateStreamSourceYAML(raw, name, src string) (string, error) {
	return editConfigYAML(raw, func(root *yaml.Node) error {
		streams := ensureMap(ensureMap(root, "go2rtc"), "streams")
		srcNode, err := yamlValueNode([]string{src})
		if err != nil {
			return err
		}
		setMapKey(streams, name, srcNode)
		return nil
	})
}

// removeCameraYAML deletes cameras[name] and go2rtc.streams[name].
func removeCameraYAML(raw, name string) (string, error) {
	return editConfigYAML(raw, func(root *yaml.Node) error {
		if cams := getMapValue(root, "cameras"); cams != nil {
			deleteMapKey(cams, name)
		}
		if g2 := getMapValue(root, "go2rtc"); g2 != nil {
			if streams := getMapValue(g2, "streams"); streams != nil {
				deleteMapKey(streams, name)
			}
		}
		return nil
	})
}

// cameraBlock builds the Frigate camera config from the stored record. The ffmpeg
// input reads from the go2rtc restream (rtsp://127.0.0.1:8554/<name>); detect,
// record (with an object-only retain mode when set), snapshots, tracked objects,
// and detection zones (with alerts/detections gated to those zones) are all driven
// by the record. Live view is auto-derived from the same-named go2rtc stream.
func cameraBlock(name string, rec devices.CameraRecord) map[string]any {
	roles := []string{"detect"} // Frigate requires a detect-role input even if detection is off
	if rec.Record {
		roles = append(roles, "record")
	}
	// Record retention. Frigate 0.17 replaced the single record.retain.{mode,days}
	// with source-split retention, so map our three UI modes onto the new keys:
	//   all            -> record.continuous.days          (full 24/7 timeline)
	//   motion         -> record.motion.days              (segments with motion)
	//   active_objects -> record.detections/alerts.retain (footage around objects)
	// Bare {enabled} otherwise (Frigate's default retention).
	record := map[string]any{"enabled": rec.Record}
	if rec.Record && rec.RecordMode != "" {
		retainDays := func() map[string]any {
			m := map[string]any{}
			if rec.RecordDays > 0 {
				m["days"] = rec.RecordDays
			}
			return m
		}
		switch rec.RecordMode {
		case "all":
			record["continuous"] = retainDays()
		case "motion":
			record["motion"] = retainDays()
		case "active_objects":
			record["detections"] = map[string]any{"retain": retainDays()}
			record["alerts"] = map[string]any{"retain": retainDays()}
		}
	}
	// Detect resolution. A 90/270 rotation makes the restreamed frame portrait, so
	// swap width/height to match (otherwise Frigate's detector and zone coords are
	// miscalibrated against the rotated image).
	dw, dh := 1280, 720
	if rec.Rotate == 90 || rec.Rotate == 270 {
		dw, dh = 720, 1280
	}
	block := map[string]any{
		"enabled": true,
		"ffmpeg": map[string]any{
			"inputs": []any{
				map[string]any{
					"path":       fmt.Sprintf("rtsp://127.0.0.1:8554/%s", name),
					"input_args": "preset-rtsp-restream",
					"roles":      roles,
				},
			},
		},
		"detect":    map[string]any{"enabled": rec.Detect, "width": dw, "height": dh},
		"record":    record,
		"snapshots": map[string]any{"enabled": rec.Snapshots},
	}
	if rec.Detect {
		block["detect"] = map[string]any{"enabled": true, "width": dw, "height": dh, "fps": 5}
		objects := rec.Objects
		if len(objects) == 0 {
			objects = []string{"person", "car"}
		}
		block["objects"] = map[string]any{"track": objects}
	}
	if zones, names := zoneBlocks(name, rec.Zones); len(zones) > 0 {
		block["zones"] = zones
		// Only alert/record for objects inside the area(s) of interest.
		block["review"] = map[string]any{
			"alerts":     map[string]any{"required_zones": names},
			"detections": map[string]any{"required_zones": names},
		}
	}
	return block
}

// zoneBlocks converts stored zones into Frigate's zones map (coordinates as a flat
// normalized "x,y,x,y…" string) plus the ordered list of valid zone names. Zones
// with a blank/duplicate/camera-colliding name or fewer than 3 points are skipped.
func zoneBlocks(camName string, zones []devices.Zone) (map[string]any, []string) {
	out := map[string]any{}
	var names []string
	seen := map[string]bool{}
	for _, z := range zones {
		zn := slugZone(z.Name)
		if zn == "" || zn == camName || seen[zn] || len(z.Coordinates) < 3 {
			continue
		}
		zb := map[string]any{"coordinates": flattenCoords(z.Coordinates)}
		if len(z.Objects) > 0 {
			zb["objects"] = z.Objects
		}
		out[zn] = zb
		names = append(names, zn)
		seen[zn] = true
	}
	return out, names
}

func flattenCoords(pairs [][]float64) string {
	parts := make([]string, 0, len(pairs)*2)
	for _, p := range pairs {
		if len(p) != 2 {
			continue
		}
		parts = append(parts, strconv.FormatFloat(p[0], 'f', 3, 64), strconv.FormatFloat(p[1], 'f', 3, 64))
	}
	return strings.Join(parts, ",")
}

// slugZone normalises a user zone name to a Frigate-safe key (lowercase alnum,
// spaces/dashes → underscore).
func slugZone(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteByte('_')
		}
	}
	return strings.Trim(b.String(), "_")
}

// editConfigYAML decodes raw into a yaml.Node tree (preserving comments), applies
// mutate to the root mapping, and re-encodes with 2-space indent.
func editConfigYAML(raw string, mutate func(root *yaml.Node) error) (string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &doc); err != nil {
		return "", fmt.Errorf("parse config: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return "", fmt.Errorf("unexpected config shape")
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return "", fmt.Errorf("config root is not a mapping")
	}
	if err := mutate(root); err != nil {
		return "", err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return "", fmt.Errorf("encode config: %w", err)
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// yamlValueNode marshals v and returns it as a value node (for grafting into the
// document tree).
func yamlValueNode(v any) (*yaml.Node, error) {
	b, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}
	var n yaml.Node
	if err := yaml.Unmarshal(b, &n); err != nil {
		return nil, err
	}
	if n.Kind != yaml.DocumentNode || len(n.Content) == 0 {
		return nil, fmt.Errorf("unexpected marshalled node")
	}
	return n.Content[0], nil
}

func getMapValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

// ensureMap returns the mapping value for key under mapping, creating an empty
// mapping if absent.
func ensureMap(mapping *yaml.Node, key string) *yaml.Node {
	if v := getMapValue(mapping, key); v != nil {
		return v
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	valNode := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	mapping.Content = append(mapping.Content, keyNode, valNode)
	return valNode
}

func setMapKey(mapping *yaml.Node, key string, val *yaml.Node) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content[i+1] = val
			return
		}
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	mapping.Content = append(mapping.Content, keyNode, val)
}

func deleteMapKey(mapping *yaml.Node, key string) bool {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			return true
		}
	}
	return false
}
