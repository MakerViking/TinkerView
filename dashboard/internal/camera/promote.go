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
	newRaw, err := insertCameraYAML(raw, name, rec.Source(), rec)
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
	newRaw, err := updateStreamSourceYAML(raw, next.Name, next.Source())
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
	}, true
}

// EditCameraSettings updates a camera's Frigate-NVR settings (detect, record mode,
// snapshots, tracked objects, detection zones). For a non-promoted camera the
// settings are just persisted and take effect when it is next promoted. For a
// promoted camera the cameras[<name>] block is rewritten and Frigate restarted
// asynchronously (mirroring editPromotedSource), reported via notices.
func (m *Manager) EditCameraSettings(name string, s devices.CameraSettings) error {
	m.mu.Lock()
	rec, ok := m.store.Camera(name)
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("unknown camera %q", name)
	}
	promoted := rec.Promoted
	if !promoted {
		// Persist now; applied on next promote.
		applySettings(&rec, s)
		err := m.store.UpsertCamera(rec)
		if err == nil {
			m.publishManaged(rec)
		}
		m.mu.Unlock()
		if err != nil {
			return fmt.Errorf("save camera: %w", err)
		}
		return nil
	}
	m.mu.Unlock()
	m.async(func(ctx context.Context) {
		if err := m.applyCameraSettings(ctx, name, s); err != nil {
			m.notice("error", err.Error())
		}
	})
	return nil
}

// applyCameraSettings rewrites a promoted camera's cameras[<name>] block from the
// new settings and restarts Frigate: backup → build → validate → persist → save →
// health-check → rollback (store + config) on failure.
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
	if !cur.Promoted {
		return fmt.Errorf("camera %q is no longer promoted", name)
	}
	next := cur
	applySettings(&next, s)
	m.notice("info", fmt.Sprintf("Applying %q settings — Frigate will restart (~25s)…", name))
	raw, err := m.frigate.GetRaw(ctx)
	if err != nil {
		return fmt.Errorf("read Frigate config: %w", err)
	}
	backupPath, err := m.frigate.backup(raw)
	if err != nil {
		return fmt.Errorf("back up Frigate config (aborting): %w", err)
	}
	newRaw, err := updateCameraBlockYAML(raw, name, next)
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
		_ = m.store.UpsertCamera(cur)
		return fmt.Errorf("save Frigate config: %w", err)
	}
	if !m.frigate.WaitHealthy(ctx, frigateHealthTimeout) {
		m.restoreBackup(ctx, backupPath)
		_ = m.store.UpsertCamera(cur)
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
	_ = m.g2.PutStream(ctx, name, rec.Source()) // resume the live tile immediately
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

// updateCameraBlockYAML replaces cameras[name] with a freshly-built block (new
// detect/record/zones from the record), leaving the go2rtc stream untouched.
func updateCameraBlockYAML(raw, name string, rec devices.CameraRecord) (string, error) {
	return editConfigYAML(raw, func(root *yaml.Node) error {
		cams := ensureMap(root, "cameras")
		block, err := yamlValueNode(cameraBlock(name, rec))
		if err != nil {
			return err
		}
		setMapKey(cams, name, block)
		return nil
	})
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
	// record-only-on-detect: an object-only retain mode keeps just the footage that
	// contains tracked objects (Frigate's recording cache + default pre-roll cover
	// the lead-up). Bare {enabled} otherwise (Frigate's default retention).
	record := map[string]any{"enabled": rec.Record}
	if rec.Record && rec.RecordMode != "" {
		retain := map[string]any{"mode": rec.RecordMode}
		if rec.RecordDays > 0 {
			retain["days"] = rec.RecordDays
		}
		record["retain"] = retain
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
		"detect":    map[string]any{"enabled": rec.Detect, "width": 1280, "height": 720},
		"record":    record,
		"snapshots": map[string]any{"enabled": rec.Snapshots},
	}
	if rec.Detect {
		block["detect"] = map[string]any{"enabled": true, "width": 1280, "height": 720, "fps": 5}
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
