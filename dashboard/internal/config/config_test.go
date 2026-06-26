package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadMissingConfig: a non-existent path is not an error — defaults are used.
func TestLoadMissingConfig(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("missing config should not error: %v", err)
	}
	if c.Listen != ":8088" {
		t.Fatalf("expected default listen, got %q", c.Listen)
	}
}

// TestLoadConfigIsDirectory reproduces the Docker/CasaOS trap: a bind-mount of a
// config FILE that doesn't exist on the host makes Docker create a DIRECTORY at the
// path. ReadFile then returns EISDIR (not IsNotExist); Load must NOT crash — it
// should fall back to defaults so the dashboard still boots.
func TestLoadConfigIsDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("a directory config path should not crash Load: %v", err)
	}
	if c.Listen != ":8088" {
		t.Fatalf("expected default listen, got %q", c.Listen)
	}
}

// TestLoadMalformedConfig: a real file with broken YAML fails loudly (user typo).
func TestLoadMalformedConfig(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte("listen: :8088\n\tbad: [unterminated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("malformed config should return an error")
	}
}

// TestLoadValidConfig parses real values over the defaults.
func TestLoadValidConfig(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte("listen: \":9099\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Listen != ":9099" {
		t.Fatalf("expected :9099, got %q", c.Listen)
	}
}
