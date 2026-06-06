package camera

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
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

	m.notice("info", fmt.Sprintf("Promoting %q to Frigate — Frigate will restart (~25s)…", name))
	raw, err := m.frigate.GetRaw(ctx)
	if err != nil {
		return fmt.Errorf("read Frigate config: %w", err)
	}
	backupPath, err := m.frigate.backup(raw)
	if err != nil {
		return fmt.Errorf("back up Frigate config (aborting): %w", err)
	}
	newRaw, err := insertCameraYAML(raw, name, rec.Src, opts)
	if err != nil {
		return fmt.Errorf("build Frigate config: %w", err)
	}
	if err := m.frigate.Validate(ctx, newRaw); err != nil {
		return fmt.Errorf("Frigate rejected the config: %w", err) // no restart happened
	}

	// Persist intent before the restart so a crash heals to the promoted state.
	rec.Promoted = true
	rec.Detect, rec.Record, rec.RecordDays, rec.Snapshots = opts.Detect, opts.Record, opts.RecordDays, opts.Snapshots
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
	_ = m.g2.PutStream(ctx, name, rec.Src) // resume the live tile immediately
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
func insertCameraYAML(raw, name, src string, opts PromoteOpts) (string, error) {
	return editConfigYAML(raw, func(root *yaml.Node) error {
		streams := ensureMap(ensureMap(root, "go2rtc"), "streams")
		srcNode, err := yamlValueNode([]string{src})
		if err != nil {
			return err
		}
		setMapKey(streams, name, srcNode)

		cams := ensureMap(root, "cameras")
		block, err := yamlValueNode(cameraBlock(name, opts))
		if err != nil {
			return err
		}
		setMapKey(cams, name, block)
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

// cameraBlock builds the Frigate camera config for a promoted camera, mirroring
// the Frigate 0.17 schema (no per-camera live.stream_name — live is auto-derived
// from the same-named go2rtc stream; no per-camera record.retain — retention is
// governed by Frigate's global record config). The input reads from the go2rtc
// restream (rtsp://127.0.0.1:8554/<name>). Security style (detect on) adds object
// tracking; printer style (detect off) just records.
func cameraBlock(name string, opts PromoteOpts) map[string]any {
	roles := []string{"detect"} // Frigate requires a detect-role input even if detection is off
	if opts.Record {
		roles = append(roles, "record")
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
		"detect":    map[string]any{"enabled": opts.Detect, "width": 1280, "height": 720},
		"record":    map[string]any{"enabled": opts.Record},
		"snapshots": map[string]any{"enabled": opts.Snapshots},
	}
	if opts.Detect {
		block["detect"] = map[string]any{"enabled": true, "width": 1280, "height": 720, "fps": 5}
		block["objects"] = map[string]any{"track": []string{"person", "car"}}
	}
	return block
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
