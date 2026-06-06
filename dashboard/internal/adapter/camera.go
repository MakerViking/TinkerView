package adapter

// CameraFeature is a togglable Frigate per-camera setting.
type CameraFeature string

const (
	FeatureDetect     CameraFeature = "detect"
	FeatureRecordings CameraFeature = "recordings"
	FeatureSnapshots  CameraFeature = "snapshots"
	FeatureEnabled    CameraFeature = "enabled"
	FeatureMotion     CameraFeature = "motion"
)

// CameraState is the normalized per-camera control state. Nil pointers mean
// "unknown" — Frigate's .../state topics are not retained, so values arrive
// asynchronously after subscribe; the UI shows an indeterminate toggle until then.
type CameraState struct {
	Name       string `json:"name"`
	Detect     *bool  `json:"detect,omitempty"`
	Recordings *bool  `json:"recordings,omitempty"`
	Snapshots  *bool  `json:"snapshots,omitempty"`
	Enabled    *bool  `json:"enabled,omitempty"`
	Motion     *bool  `json:"motion,omitempty"`
	// RecordConfigured is the config ceiling from GET /api/config: false → the
	// record toggle is a no-op and the UI renders it disabled. Pointer + omitempty
	// so partial updates from other publishers (go2rtc enumeration, toggle-state
	// echoes) don't clobber a known value; nil means "unknown".
	RecordConfigured *bool `json:"recordConfigured,omitempty"`
	// HasHQ is set when an enhanced "<name>_hq" go2rtc variant exists, so the UI
	// can offer a per-tile HQ toggle. Pointer + omitempty for the same
	// non-clobbering reason as RecordConfigured.
	HasHQ *bool `json:"hasHq,omitempty"`
	// Managed is set (by the CameraManager) for cameras the dashboard owns via the
	// device store, so the UI may offer edit/remove/promote. Cameras that only
	// exist in the hand-authored frigate config leave this nil → read-only in the UI.
	Managed *bool `json:"managed,omitempty"`
	// Promoted is set (by the CameraManager) when a managed camera has been written
	// into Frigate's config (NVR). nil/false = live-tile only. Same non-clobbering
	// pointer-merge reason as the others.
	Promoted *bool `json:"promoted,omitempty"`
	// Deleted signals the camera no longer exists (the hub removes it and tells
	// clients to drop the tile).
	Deleted bool `json:"-"`
}

// Alert is a normalized notification surfaced in the in-app feed (independent of
// ntfy). Kind drives the icon/colour.
type Alert struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"` // person|motion|camera_offline|print_done|print_failed|frigate_down
	Camera    string `json:"camera,omitempty"`
	PrinterID string `json:"printerId,omitempty"`
	Ts        int64  `json:"ts"` // unix ms
	Title     string `json:"title"`
	Message   string `json:"message"`
	ThumbURL  string `json:"thumbUrl,omitempty"`
	ClipURL   string `json:"clipUrl,omitempty"`
}
