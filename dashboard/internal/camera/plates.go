package camera

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"gopkg.in/yaml.v3"
)

// Known-plates (LPR) management. Frigate has no runtime API for known plates —
// they live in the global `lpr.known_plates` config block (name → [plate strings/
// regexes]) — so enrolling a plate means editing Frigate's config and restarting,
// mirroring the promote flow (backup → build → validate → save → health → rollback).

// KnownPlates reads the current `lpr.known_plates` map from Frigate's config.
// Returns an empty (non-nil) map when none are configured.
func (m *Manager) KnownPlates(ctx context.Context) (map[string][]string, error) {
	if m.frigate == nil {
		return nil, fmt.Errorf("Frigate config editing is not available")
	}
	raw, err := m.frigate.GetRaw(ctx)
	if err != nil {
		return nil, fmt.Errorf("read Frigate config: %w", err)
	}
	return parseKnownPlates(raw)
}

// SetKnownPlates validates the new plate map quickly, then rewrites
// `lpr.known_plates` and restarts Frigate asynchronously (reported via notices).
func (m *Manager) SetKnownPlates(plates map[string][]string) error {
	if m.frigate == nil {
		return fmt.Errorf("Frigate config editing is not available")
	}
	clean, err := cleanPlates(plates)
	if err != nil {
		return err
	}
	m.async(func(ctx context.Context) {
		if err := m.applyKnownPlates(ctx, clean); err != nil {
			m.notice("error", err.Error())
		}
	})
	return nil
}

func (m *Manager) applyKnownPlates(ctx context.Context, plates map[string][]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.frigate == nil {
		return fmt.Errorf("Frigate config editing is not available")
	}
	m.notice("info", "Updating known plates — Frigate will restart (~25s)…")
	raw, err := m.frigate.GetRaw(ctx)
	if err != nil {
		return fmt.Errorf("read Frigate config: %w", err)
	}
	backupPath, err := m.frigate.backup(raw)
	if err != nil {
		return fmt.Errorf("back up Frigate config (aborting): %w", err)
	}
	newRaw, err := setKnownPlatesYAML(raw, plates)
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
		return fmt.Errorf("Frigate did not come back after the known-plates change; restored previous config (backup: %s)", backupPath)
	}
	m.notice("info", fmt.Sprintf("Known plates updated (%d)", len(plates)))
	slog.Info("known plates updated", "count", len(plates))
	return nil
}

// cleanPlates trims/validates the map: drops blank names and blank plate strings,
// dedupes, and rejects an entry left with no plates.
func cleanPlates(in map[string][]string) (map[string][]string, error) {
	out := make(map[string][]string, len(in))
	for name, plates := range in {
		n := strings.TrimSpace(name)
		if n == "" {
			return nil, fmt.Errorf("a plate owner name is required")
		}
		seen := map[string]bool{}
		var list []string
		for _, p := range plates {
			p = strings.TrimSpace(p)
			if p == "" || seen[strings.ToUpper(p)] {
				continue
			}
			seen[strings.ToUpper(p)] = true
			list = append(list, p)
		}
		if len(list) == 0 {
			return nil, fmt.Errorf("%q has no plates", n)
		}
		out[n] = list
	}
	return out, nil
}

// parseKnownPlates extracts lpr.known_plates from raw config YAML. A value may be a
// single string or a list of strings (Frigate accepts both); both normalize to a
// slice.
func parseKnownPlates(raw string) (map[string][]string, error) {
	var doc struct {
		LPR struct {
			KnownPlates map[string]stringOrSlice `yaml:"known_plates"`
		} `yaml:"lpr"`
	}
	if err := yaml.Unmarshal([]byte(raw), &doc); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	out := make(map[string][]string, len(doc.LPR.KnownPlates))
	for name, v := range doc.LPR.KnownPlates {
		out[name] = []string(v)
	}
	return out, nil
}

// stringOrSlice unmarshals a YAML scalar or sequence of scalars into a string
// slice, so `known_plates: {Bob: ABC123}` and `{Bob: [ABC123, DEF456]}` both work.
type stringOrSlice []string

func (s *stringOrSlice) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		*s = []string{n.Value}
		return nil
	}
	var list []string
	if err := n.Decode(&list); err != nil {
		return err
	}
	*s = list
	return nil
}

// setKnownPlatesYAML rewrites lpr.known_plates with the given map (comment-
// preserving). An empty map removes the known_plates key entirely. yaml.v3
// marshals map keys in sorted order, so the diff is stable.
func setKnownPlatesYAML(raw string, plates map[string][]string) (string, error) {
	return editConfigYAML(raw, func(root *yaml.Node) error {
		lpr := ensureMap(root, "lpr")
		if len(plates) == 0 {
			deleteMapKey(lpr, "known_plates")
			return nil
		}
		node, err := yamlValueNode(plates)
		if err != nil {
			return err
		}
		setMapKey(lpr, "known_plates", node)
		return nil
	})
}
