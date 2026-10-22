package devices

import (
	"encoding/json"
	"strings"
)

// PrinterView is the client-facing shape of a stored printer: every field of
// PrinterRecord EXCEPT the secrets (API key / Bambu access code), which are
// replaced by presence flags. It is what GET /api/printers/{id} returns and what
// PrinterRecord marshals to, so a secret can never leak by serializing a record.
type PrinterView struct {
	ID            string   `json:"id"`
	Kind          string   `json:"kind"`
	Name          string   `json:"name"`
	BaseURL       string   `json:"baseUrl"`
	IP            string   `json:"ip"`
	Serial        string   `json:"serial"`
	Model         string   `json:"model"`
	Camera        string   `json:"camera"`
	UIURL         string   `json:"uiUrl"`
	Macros        []string `json:"macros"`
	HasAPIKey     bool     `json:"hasApiKey"`
	HasAccessCode bool     `json:"hasAccessCode"`
	Managed       bool     `json:"managed"`
}

// View returns the redacted client-facing form of the record. A record that
// exists in the store is by definition UI-managed.
func (r PrinterRecord) View() PrinterView {
	macros := r.Macros
	if macros == nil {
		macros = []string{}
	}
	return PrinterView{
		ID: r.ID, Kind: r.Kind, Name: r.Name, BaseURL: r.BaseURL, IP: r.IP,
		Serial: r.Serial, Model: r.Model, Camera: r.Camera, UIURL: r.UIURL,
		Macros:        macros,
		HasAPIKey:     r.APIKey != "",
		HasAccessCode: r.AccessCode != "",
		Managed:       true,
	}
}

// MarshalJSON never emits the API key or access code (REVIEW 1.2). Decoding is
// untouched: request bodies still populate apiKey/accessCode via the struct tags.
// (YAML persistence is separate and keeps the secrets.)
func (r PrinterRecord) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.View())
}

// InferSrcKind recovers the UI source kind of a stored go2rtc source string for
// records written before CameraRecord.SrcKind existed. It is the inverse of the
// camera manager's resolveSrc for the shapes that function emits; anything it
// does not recognise is "advanced" (edited verbatim).
func InferSrcKind(src string) string {
	switch {
	case strings.HasPrefix(src, "ffmpeg:"):
		rest := strings.TrimPrefix(src, "ffmpeg:")
		i := strings.IndexByte(rest, '#')
		if i < 0 {
			return "advanced"
		}
		switch rest[i:] {
		case "#video=h264#hardware":
			return "mjpeg"
		case "#video=h264":
			return "h264"
		}
		return "advanced"
	case strings.HasPrefix(src, "rtsp://"), strings.HasPrefix(src, "rtsps://"):
		return "rtsp"
	case strings.HasPrefix(src, "http://"), strings.HasPrefix(src, "https://"):
		return "mjpeg_raw"
	}
	return "advanced"
}

// SrcInputOf extracts the user-facing source URL from a stored go2rtc source for
// the given kind: the ffmpeg: wrapper and its #video=h264[#hardware] suffix are
// removed for h264/mjpeg, and any URL userinfo (credentials) is stripped.
func SrcInputOf(kind, src string) string {
	if kind == "h264" || kind == "mjpeg" {
		src = strings.TrimPrefix(src, "ffmpeg:")
		if i := strings.IndexByte(src, '#'); i >= 0 {
			src = src[:i]
		}
	}
	return StripUserinfo(src)
}

// SplitUserinfo splits the first scheme://userinfo@host in src (also behind a
// leading "ffmpeg:") without URL parsing, so placeholders such as
// "{FRIGATE_RTSP_USER}" in a hand-authored Frigate config survive. head is the
// text up to and including "://", userinfo is the "user[:pass]" part ("" if
// none), tail is everything from the host on.
func SplitUserinfo(src string) (head, userinfo, tail string) {
	i := strings.Index(src, "://")
	if i < 0 {
		return src, "", ""
	}
	head, rest := src[:i+3], src[i+3:]
	end := len(rest)
	if j := strings.IndexAny(rest, "/?#"); j >= 0 {
		end = j
	}
	if at := strings.LastIndexByte(rest[:end], '@'); at >= 0 {
		return head, rest[:at], rest[at+1:]
	}
	return head, "", rest
}

// StripUserinfo removes any credentials embedded in the source URL.
func StripUserinfo(src string) string {
	head, ui, tail := SplitUserinfo(src)
	if ui == "" {
		return src
	}
	return head + tail
}

// WithUserinfo returns src with its userinfo replaced by ui (removed if empty).
func WithUserinfo(src, ui string) string {
	head, _, tail := SplitUserinfo(src)
	if tail == "" && !strings.Contains(src, "://") {
		return src
	}
	if ui == "" {
		return head + tail
	}
	return head + ui + "@" + tail
}
