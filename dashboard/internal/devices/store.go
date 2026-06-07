// Package devices is the dashboard's writable, UI-managed device registry. The
// static config.yaml is mounted read-only, so devices added/edited/removed from
// the UI are persisted here (a YAML file under the rw /data volume) instead.
// The Store is pure persistence; the Manager (manager.go) owns lifecycle and the
// CameraManager (internal/camera) owns the go2rtc/Frigate side effects.
package devices

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// schemaVersion is bumped when the on-disk layout changes; Open tolerates 0
// (pre-versioning) as the current version.
const schemaVersion = 1

// StoreData is the on-disk document.
type StoreData struct {
	Version  int             `yaml:"version"`
	Printers []PrinterRecord `yaml:"printers"`
	Cameras  []CameraRecord  `yaml:"cameras"`
}

// PrinterRecord mirrors config.Printer field-for-field so a static seed converts
// with no loss (the conversion lives in cmd/dashboard, which imports both, to
// keep this package a leaf with no config dependency).
type PrinterRecord struct {
	ID         string `yaml:"id" json:"id"`
	Kind       string `yaml:"kind" json:"kind"`
	Name       string `yaml:"name" json:"name"`
	BaseURL    string `yaml:"base_url" json:"baseUrl"`
	APIKey     string `yaml:"api_key" json:"apiKey"`
	IP         string `yaml:"ip" json:"ip"`
	Serial     string `yaml:"serial" json:"serial"`
	AccessCode string `yaml:"access_code" json:"accessCode"`
	Model      string `yaml:"model" json:"model"`
	Camera     string `yaml:"camera" json:"camera"`
	UIURL      string `yaml:"ui_url" json:"uiUrl"`
	// Macros is the curated allowlist of gcode macros exposed as buttons in the UI
	// (e.g. T0..T3, M600). Empty → the adapter advertises a safe default set.
	Macros []string `yaml:"macros,omitempty" json:"macros,omitempty"`
}

// CameraRecord is a UI-added camera. Src is the resolved go2rtc source string
// (raw URL or an ffmpeg:/exec: form). When Promoted, Frigate owns the stream via
// its own config and the reconcile loop leaves it alone; the Detect/Record/...
// fields are the options used to build the Frigate camera block.
type CameraRecord struct {
	Name string `yaml:"name" json:"name"`
	Src  string `yaml:"src" json:"src"`
	// Username/Password, when set, are injected into Src's stream URL by Source()
	// so the credentials live in one place instead of being embedded in the URL.
	// Stored in plaintext like printer API keys; Password is never sent to clients.
	Username   string `yaml:"username,omitempty" json:"username,omitempty"`
	Password   string `yaml:"password,omitempty" json:"-"`
	Promoted   bool   `yaml:"promoted" json:"promoted"`
	Detect     bool   `yaml:"detect,omitempty" json:"detect,omitempty"`
	Record     bool   `yaml:"record,omitempty" json:"record,omitempty"`
	RecordDays int    `yaml:"record_days,omitempty" json:"recordDays,omitempty"`
	Snapshots  bool   `yaml:"snapshots,omitempty" json:"snapshots,omitempty"`
}

// Source returns the go2rtc source for this camera with its credentials injected
// into the stream URL's userinfo (replacing any already present). With no
// username set it returns Src verbatim, so a camera whose URL already embeds
// credentials keeps working. It understands a leading "ffmpeg:" pipeline wrapper
// and a trailing "#…" go2rtc options fragment, and leaves non-URL sources
// (exec:, raw "advanced" strings) untouched.
func (r CameraRecord) Source() string {
	return injectCredentials(r.Src, r.Username, r.Password)
}

func injectCredentials(src, user, pass string) string {
	if user == "" && pass == "" {
		return src
	}
	prefix, rest := "", src
	switch {
	case strings.HasPrefix(rest, "ffmpeg:"):
		prefix, rest = "ffmpeg:", strings.TrimPrefix(rest, "ffmpeg:")
	case strings.HasPrefix(rest, "exec:"):
		return src // raw command line, not a URL
	}
	urlPart, frag := rest, ""
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		urlPart, frag = rest[:i], rest[i:]
	}
	u, err := url.Parse(urlPart)
	if err != nil || u.Host == "" {
		return src // not a single parseable URL — leave as-is
	}
	u.User = url.UserPassword(user, pass)
	return prefix + u.String() + frag
}

// Store persists StoreData to a single YAML file, guarded by a mutex. It is the
// sole writer of that file.
type Store struct {
	path string
	mu   sync.Mutex
	data StoreData
}

// Open loads the store from path. A missing file yields an empty store (the
// first save creates it); a malformed file is a hard error so we never silently
// drop a user's devices.
func Open(path string) (*Store, error) {
	s := &Store{path: path, data: StoreData{Version: schemaVersion}}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(b, &s.data); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if s.data.Version == 0 {
		s.data.Version = schemaVersion
	}
	return s, nil
}

// Empty reports whether the store has no devices (used to gate the first-run
// import of static config.yaml printers).
func (s *Store) Empty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.data.Printers) == 0 && len(s.data.Cameras) == 0
}

// Printers returns a copy of the stored printers.
func (s *Store) Printers() []PrinterRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]PrinterRecord(nil), s.data.Printers...)
}

// Printer returns the stored record for id (and whether it exists).
func (s *Store) Printer(id string) (PrinterRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.data.Printers {
		if p.ID == id {
			return p, true
		}
	}
	return PrinterRecord{}, false
}

// Cameras returns a copy of the stored cameras.
func (s *Store) Cameras() []CameraRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]CameraRecord(nil), s.data.Cameras...)
}

// Camera returns the stored record for name (and whether it exists).
func (s *Store) Camera(name string) (CameraRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.data.Cameras {
		if c.Name == name {
			return c, true
		}
	}
	return CameraRecord{}, false
}

// UpsertPrinter inserts or replaces a printer by ID and persists.
func (s *Store) UpsertPrinter(r PrinterRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Printers {
		if s.data.Printers[i].ID == r.ID {
			s.data.Printers[i] = r
			return s.saveLocked()
		}
	}
	s.data.Printers = append(s.data.Printers, r)
	return s.saveLocked()
}

// DeletePrinter removes a printer by ID and persists. Returns whether it existed.
func (s *Store) DeletePrinter(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Printers {
		if s.data.Printers[i].ID == id {
			s.data.Printers = append(s.data.Printers[:i], s.data.Printers[i+1:]...)
			return true, s.saveLocked()
		}
	}
	return false, nil
}

// UpsertCamera inserts or replaces a camera by Name and persists.
func (s *Store) UpsertCamera(r CameraRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Cameras {
		if s.data.Cameras[i].Name == r.Name {
			s.data.Cameras[i] = r
			return s.saveLocked()
		}
	}
	s.data.Cameras = append(s.data.Cameras, r)
	return s.saveLocked()
}

// DeleteCamera removes a camera by Name and persists. Returns whether it existed.
func (s *Store) DeleteCamera(name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.data.Cameras {
		if s.data.Cameras[i].Name == name {
			s.data.Cameras = append(s.data.Cameras[:i], s.data.Cameras[i+1:]...)
			return true, s.saveLocked()
		}
	}
	return false, nil
}

// saveLocked atomically writes the store: a temp file in the same directory is
// written + fsynced, then renamed over the target (atomic on the same
// filesystem). Caller holds s.mu.
func (s *Store) saveLocked() error {
	s.data.Version = schemaVersion
	b, err := yaml.Marshal(&s.data)
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".devices-*.yaml")
	if err != nil {
		return fmt.Errorf("create temp in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path)
}
