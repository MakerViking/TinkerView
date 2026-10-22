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

// Zone is a Frigate detection zone (area of interest): a polygon over the frame
// where the listed objects count. Coordinates are normalized [x,y] pairs (0..1).
type Zone struct {
	Name        string      `yaml:"name" json:"name"`
	Coordinates [][]float64 `yaml:"coordinates" json:"coordinates"`
	Objects     []string    `yaml:"objects,omitempty" json:"objects,omitempty"`
}

// CameraSettings is the editable Frigate-NVR configuration for a camera, applied
// to the cameras[<name>] block (carried by the camera.settings command).
type CameraSettings struct {
	Detect     bool     `json:"detect"`
	Record     bool     `json:"record"`
	RecordMode string   `json:"recordMode"` // active_objects | motion | all
	RecordDays int      `json:"recordDays"`
	Snapshots  bool     `json:"snapshots"`
	Objects    []string `json:"objects,omitempty"`
	Zones      []Zone   `json:"zones,omitempty"`
	// Image transform (applied at the go2rtc source via an ffmpeg transcode, so it
	// affects the live tile, recordings, snapshots and detection alike).
	Rotate int  `json:"rotate,omitempty"` // 0|90|180|270 (clockwise degrees)
	FlipH  bool `json:"flipH,omitempty"`
	FlipV  bool `json:"flipV,omitempty"`
}

// DetectionSettings is the normalized, UI-editable Frigate object-detection config:
// which detector + device runs detection, the model it loads, and the ffmpeg
// hardware-DECODE preset. It maps to Frigate's top-level detectors:/model:/ffmpeg:
// keys (see internal/camera/detection.go). Advanced is set when the live config
// doesn't match one of our known templates (hand-tuned), so the UI warns before
// overwriting rather than silently clobbering it.
type DetectionSettings struct {
	Detector string `json:"detector"`          // cpu|openvino|edgetpu|onnx|rocm|hailo8l
	Device   string `json:"device,omitempty"`  // openvino: GPU|NPU|CPU|AUTO ; edgetpu: usb|pci
	Hwaccel  string `json:"hwaccel,omitempty"` // "" | vaapi | qsv-h264 | qsv-h265 | nvidia | rpi-64-h264
	// Model is used by the onnx/rocm detectors (the user supplies an exported .onnx);
	// openvino uses the bundled model, and cpu/edgetpu/hailo use their bundled models
	// (these leave the model fields blank).
	ModelType     string `json:"modelType,omitempty"`
	ModelWidth    int    `json:"modelWidth,omitempty"`
	ModelHeight   int    `json:"modelHeight,omitempty"`
	ModelDtype    string `json:"modelDtype,omitempty"` // float | float16
	ModelPath     string `json:"modelPath,omitempty"`
	ModelLabelmap string `json:"modelLabelmap,omitempty"`
	Advanced      bool   `json:"advanced,omitempty"`
}

// CameraRecord is a UI-added camera. Src is the resolved go2rtc source string
// (raw URL or an ffmpeg:/exec: form). When Promoted, Frigate owns the stream via
// its own config and the reconcile loop leaves it alone; the Detect/Record/...
// fields are the options used to build the Frigate camera block.
type CameraRecord struct {
	Name string `yaml:"name" json:"name"`
	Src  string `yaml:"src" json:"src"`
	// SrcKind is the UI source kind Src was resolved from (rtsp|h264|mjpeg|
	// mjpeg_raw|advanced). Records written before it existed leave it empty;
	// InferSrcKind recovers it from the Src prefix.
	SrcKind string `yaml:"src_kind,omitempty" json:"srcKind,omitempty"`
	// Username/Password, when set, are injected into Src's stream URL by Source()
	// so the credentials live in one place instead of being embedded in the URL.
	// Stored in plaintext like printer API keys; Password is never sent to clients.
	Username string `yaml:"username,omitempty" json:"username,omitempty"`
	Password string `yaml:"password,omitempty" json:"-"`
	Promoted bool   `yaml:"promoted" json:"promoted"`
	// ConfigStream marks a camera whose go2rtc stream is defined in Frigate's config
	// (a hand-authored camera "adopted" into the store to apply an image transform),
	// as opposed to a UI-added camera served as a runtime go2rtc stream. When set,
	// the reconcile loop won't re-PUT it at runtime and stream edits go through the
	// Frigate config + restart. Promoted cameras are always config streams too.
	ConfigStream bool     `yaml:"config_stream,omitempty" json:"-"`
	Detect       bool     `yaml:"detect,omitempty" json:"detect,omitempty"`
	Record       bool     `yaml:"record,omitempty" json:"record,omitempty"`
	RecordDays   int      `yaml:"record_days,omitempty" json:"recordDays,omitempty"`
	Snapshots    bool     `yaml:"snapshots,omitempty" json:"snapshots,omitempty"`
	RecordMode   string   `yaml:"record_mode,omitempty" json:"recordMode,omitempty"` // active_objects|motion|all
	Objects      []string `yaml:"objects,omitempty" json:"objects,omitempty"`        // tracked objects; default person,car
	Zones        []Zone   `yaml:"zones,omitempty" json:"zones,omitempty"`
	// Image transform, re-applied to the go2rtc source by BuildTransformSource. Src
	// always stays the pristine base; these fields drive the wrapper so editing is
	// idempotent (resetting to 0/no-flip removes the transcode wrapper entirely).
	Rotate int  `yaml:"rotate,omitempty" json:"rotate,omitempty"` // 0|90|180|270 (clockwise degrees)
	FlipH  bool `yaml:"flip_h,omitempty" json:"flipH,omitempty"`
	FlipV  bool `yaml:"flip_v,omitempty" json:"flipV,omitempty"`
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

// BuildTransformSource wraps a credential-injected go2rtc source so it is rotated/
// flipped via an ffmpeg transcode. rotate is clockwise degrees (0|90|180|270); the
// filters are applied rotate-first then flips, joined into a single ffmpeg "-vf"
// chain emitted as "#video=h264#raw=-vf <chain>" (+"#hardware" on a GPU host). A
// base already starting "ffmpeg:" has the params merged onto it rather than nested.
//
// It returns base unchanged when no transform is requested, and an error when the
// transform can't be applied: an "exec:" or unparseable base, or a base that
// already carries a "raw="/"rotate=" go2rtc param (a second "-vf" would conflict).
func BuildTransformSource(base string, rotate int, flipH, flipV, hardware bool) (string, error) {
	chain := transformChain(rotate, flipH, flipV)
	if chain == "" {
		return base, nil // no-op: leave the base pristine
	}
	if strings.HasPrefix(base, "exec:") {
		return "", fmt.Errorf("image rotate/flip needs an ffmpeg transcode, which isn't possible for an exec: source")
	}

	// Split off an existing "ffmpeg:" prefix and "#…" option fragment. We always
	// re-emit an "ffmpeg:" source below, so the prefix itself is just stripped.
	rest := strings.TrimPrefix(base, "ffmpeg:")
	input, fragment := rest, ""
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		input, fragment = rest[:i], rest[i:]
	}
	if input == "" {
		return "", fmt.Errorf("camera source is empty")
	}

	// Existing params (everything after the input). Reject a source that already
	// uses a raw/-vf filter or a rotate param — we'd produce a conflicting second
	// "-vf" and silently corrupt the pipeline.
	params := splitParams(fragment)
	hasVideoH264 := false
	for _, p := range params {
		switch {
		case strings.HasPrefix(p, "raw=") || strings.HasPrefix(p, "rotate="):
			return "", fmt.Errorf("this camera's source already uses a raw ffmpeg filter; clear it before using image controls")
		case p == "video=h264":
			hasVideoH264 = true
		case p == "hardware":
			hardware = false // already present; don't duplicate
		}
	}

	out := "ffmpeg:" + input
	for _, p := range params {
		out += "#" + p
	}
	if !hasVideoH264 {
		out += "#video=h264"
	}
	out += "#raw=-vf " + chain
	if hardware {
		out += "#hardware"
	}
	return out, nil
}

// splitParams splits a leading "#a#b#c" option fragment into ["a","b","c"].
func splitParams(fragment string) []string {
	fragment = strings.TrimPrefix(fragment, "#")
	if fragment == "" {
		return nil
	}
	return strings.Split(fragment, "#")
}

// transformChain returns the ffmpeg "-vf" filter chain for the given rotate/flip,
// or "" when the transform is the identity (rotate 0, no flips). rotate is
// clockwise degrees; filters are rotate-first then flips.
func transformChain(rotate int, flipH, flipV bool) string {
	var parts []string
	switch rotate {
	case 90:
		parts = append(parts, "transpose=1") // 90° clockwise
	case 180:
		parts = append(parts, "transpose=2", "transpose=2")
	case 270:
		parts = append(parts, "transpose=2") // 90° counter-clockwise
	}
	if flipH {
		parts = append(parts, "hflip")
	}
	if flipV {
		parts = append(parts, "vflip")
	}
	return strings.Join(parts, ",")
}

// Store persists StoreData to a single YAML file, guarded by a mutex. It is the
// sole writer of that file.
type Store struct {
	path string
	mu   sync.Mutex
	data StoreData
	// refHook, when set, is told which printers' camera reference changed.
	refHook func(ids []string)
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

// mutateLocked applies fn to the data and persists. On a save failure the
// in-memory data is restored to its pre-call state, so memory never diverges
// from disk (a failed write must not leave a ghost device). Caller holds s.mu.
func (s *Store) mutateLocked(fn func(d *StoreData)) error {
	prev := StoreData{
		Version:  s.data.Version,
		Printers: append([]PrinterRecord(nil), s.data.Printers...),
		Cameras:  append([]CameraRecord(nil), s.data.Cameras...),
	}
	fn(&s.data)
	if err := s.saveLocked(); err != nil {
		s.data = prev
		return err
	}
	return nil
}

// UpsertPrinter inserts or replaces a printer by ID and persists.
func (s *Store) UpsertPrinter(r PrinterRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateLocked(func(d *StoreData) {
		for i := range d.Printers {
			if d.Printers[i].ID == r.ID {
				d.Printers[i] = r
				return
			}
		}
		d.Printers = append(d.Printers, r)
	})
}

// DeletePrinter removes a printer by ID and persists. Returns whether it existed.
func (s *Store) DeletePrinter(id string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	found := false
	err := s.mutateLocked(func(d *StoreData) {
		for i := range d.Printers {
			if d.Printers[i].ID == id {
				d.Printers = append(d.Printers[:i:i], d.Printers[i+1:]...)
				found = true
				return
			}
		}
	})
	if err != nil {
		return false, err
	}
	return found, nil
}

// UpsertCamera inserts or replaces a camera by Name and persists.
func (s *Store) UpsertCamera(r CameraRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mutateLocked(func(d *StoreData) {
		for i := range d.Cameras {
			if d.Cameras[i].Name == r.Name {
				d.Cameras[i] = r
				return
			}
		}
		d.Cameras = append(d.Cameras, r)
	})
}

// DeleteCamera removes a camera by Name and persists. Returns whether it existed.
func (s *Store) DeleteCamera(name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	found := false
	err := s.mutateLocked(func(d *StoreData) {
		for i := range d.Cameras {
			if d.Cameras[i].Name == name {
				d.Cameras = append(d.Cameras[:i:i], d.Cameras[i+1:]...)
				found = true
				return
			}
		}
	})
	if err != nil {
		return false, err
	}
	return found, nil
}

// SetPrinterRefHook registers fn to be called (outside the store lock) with the
// ids of printers whose Camera reference was rewritten by RenameCamera or
// DeleteCameraClearRefs, so the live adapters can be reloaded.
func (s *Store) SetPrinterRefHook(fn func(ids []string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refHook = fn
}

func (s *Store) fireRefHook(ids []string) {
	if len(ids) == 0 {
		return
	}
	s.mu.Lock()
	fn := s.refHook
	s.mu.Unlock()
	if fn != nil {
		fn(ids)
	}
}

// RenameCamera replaces the record stored under oldName with rec (whose Name is
// the new name) and rewrites every printer's Camera reference oldName -> rec.Name,
// all in ONE save: either the whole rename lands or none of it does. It returns
// the ids of the printers whose reference changed.
func (s *Store) RenameCamera(oldName string, rec CameraRecord) ([]string, error) {
	s.mu.Lock()
	var affected []string
	err := func() error {
		defer s.mu.Unlock()
		found := false
		for _, c := range s.data.Cameras {
			if c.Name == oldName {
				found = true
			} else if c.Name == rec.Name {
				return fmt.Errorf("a camera named %q already exists", rec.Name)
			}
		}
		if !found {
			return fmt.Errorf("unknown camera %q", oldName)
		}
		return s.mutateLocked(func(d *StoreData) {
			for i := range d.Cameras {
				if d.Cameras[i].Name == oldName {
					d.Cameras[i] = rec
					break
				}
			}
			affected = retargetPrinters(d, oldName, rec.Name)
		})
	}()
	if err != nil {
		return nil, err
	}
	s.fireRefHook(affected)
	return affected, nil
}

// DeleteCameraClearRefs removes a camera and blanks the Camera reference of every
// printer that pointed at it, in one save. Returns whether the camera existed and
// the ids of the printers that were updated.
func (s *Store) DeleteCameraClearRefs(name string) (bool, []string, error) {
	s.mu.Lock()
	found := false
	var affected []string
	err := s.mutateLocked(func(d *StoreData) {
		for i := range d.Cameras {
			if d.Cameras[i].Name == name {
				d.Cameras = append(d.Cameras[:i:i], d.Cameras[i+1:]...)
				found = true
				break
			}
		}
		if found {
			affected = retargetPrinters(d, name, "")
		}
	})
	s.mu.Unlock()
	if err != nil {
		return false, nil, err
	}
	s.fireRefHook(affected)
	return found, affected, nil
}

// retargetPrinters rewrites printers' Camera == from to `to` (copy-on-write so a
// rollback snapshot is unaffected) and returns the changed printer ids.
func retargetPrinters(d *StoreData, from, to string) []string {
	var ids []string
	for i := range d.Printers {
		if d.Printers[i].Camera == from {
			d.Printers[i].Camera = to
			ids = append(ids, d.Printers[i].ID)
		}
	}
	return ids
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
