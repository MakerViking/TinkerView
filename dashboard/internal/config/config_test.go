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

// TestResolveExplicitWins: the -config flag beats every candidate.
func TestResolveExplicitWins(t *testing.T) {
	t.Setenv("DASHBOARD_DATA_DIR", t.TempDir())
	if got := Resolve("/somewhere/else.yaml"); got != "/somewhere/else.yaml" {
		t.Fatalf("explicit path should win, got %q", got)
	}
}

// TestResolvePicksDataDirFile: with a real file in the data dir, that's the config.
func TestResolvePicksDataDirFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DASHBOARD_DATA_DIR", dir)
	want := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(want, []byte("listen: \":9099\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Resolve(""); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

// TestResolveSkipsDirectoryCandidate is the fix for the first-run trap: a leftover
// directory (Docker's stand-in for a bind-mounted file missing on the host) is not
// a config file. Resolve must not treat it as one — it names the primary path so
// SeedTemplate can write a real file there instead.
func TestResolveSkipsDirectoryCandidate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DASHBOARD_DATA_DIR", dir)
	primary := filepath.Join(dir, "config.yaml")
	if err := os.Mkdir(primary, 0o755); err != nil {
		t.Fatal(err)
	}
	// Only candidate is the directory, so Resolve falls through to the primary
	// path; Load then survives it (see TestLoadConfigIsDirectory).
	if got := Resolve(""); got != primary {
		t.Fatalf("expected %q, got %q", primary, got)
	}
	c, err := Load(Resolve(""))
	if err != nil {
		t.Fatalf("resolved directory must not crash Load: %v", err)
	}
	if c.Listen != ":8088" {
		t.Fatalf("expected defaults, got %q", c.Listen)
	}
}

// TestSeedTemplate: first run writes the template, and loading it changes nothing
// (every line is commented). A second call must not clobber the user's edits.
func TestSeedTemplate(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	SeedTemplate(p)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("template not written: %v", err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatalf("seeded template must parse: %v", err)
	}
	if c.Listen != ":8088" || len(c.Printers) != 0 {
		t.Fatalf("seeded template must be inert, got listen=%q printers=%d", c.Listen, len(c.Printers))
	}

	edited := []byte("listen: \":9099\"\n")
	if err := os.WriteFile(p, edited, 0o644); err != nil {
		t.Fatal(err)
	}
	SeedTemplate(p)
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(edited) {
		t.Fatal("SeedTemplate overwrote an existing config")
	}
}

// TestSeedTemplateLeavesDirectoryAlone: never try to write over a leftover
// directory — best-effort means silent, not destructive.
func TestSeedTemplateLeavesDirectoryAlone(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.Mkdir(p, 0o755); err != nil {
		t.Fatal(err)
	}
	SeedTemplate(p)
	st, err := os.Stat(p)
	if err != nil || !st.IsDir() {
		t.Fatalf("directory should be left untouched (err=%v)", err)
	}
}
