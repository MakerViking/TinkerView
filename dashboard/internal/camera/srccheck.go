package camera

import (
	"fmt"
	"strings"
)

// netSchemes are the transport schemes an ffmpeg: source may read from. Anything
// else after "ffmpeg:" (a bare path, device, lavfi graph, file:, concat:, ...)
// would let a UI-supplied source read local files or synthesize input.
var netSchemes = []string{"http://", "https://", "rtsp://", "rtsps://", "rtmp://", "rtmps://"}

// checkUserSource rejects UI-provided go2rtc sources that spawn a process or read
// local resources inside the go2rtc container: the exec:, echo: and expr:
// schemes, and ffmpeg: sources whose input isn't a network URL or that pass
// raw input/global ffmpeg arguments. Other schemes (onvif://, tapo://, ...) are
// left to the power user. This only guards sources typed into the dashboard; a
// stream already in the Frigate config is never re-checked.
func checkUserSource(src string) error {
	for _, r := range src {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("source contains a control character")
		}
	}
	lower := strings.ToLower(strings.TrimSpace(src))
	switch {
	case strings.HasPrefix(lower, "exec:"), strings.HasPrefix(lower, "echo:"), strings.HasPrefix(lower, "expr:"):
		return fmt.Errorf("exec:, echo: and expr: sources are not allowed (they run commands inside go2rtc)")
	case strings.HasPrefix(lower, "ffmpeg:"):
		inner := strings.TrimPrefix(lower, "ffmpeg:")
		ok := false
		for _, s := range netSchemes {
			if strings.HasPrefix(inner, s) {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("ffmpeg: sources must read from an http(s), rtsp(s) or rtmp(s) URL")
		}
		// go2rtc splits the ffmpeg command line on spaces, so the URL part must be a
		// single token: whitespace or quotes would smuggle in extra arguments.
		urlPart, params, _ := strings.Cut(inner, "#")
		if strings.ContainsAny(urlPart, " \t\"'`\\") {
			return fmt.Errorf("ffmpeg: source URL may not contain spaces, quotes or backslashes")
		}
		// Fragment params: custom input=/global= args replace or prepend ffmpeg's own.
		for _, p := range strings.Split(params, "#") {
			custom := func(prefix string) bool { // named presets (rtsp/udp) are fine; raw args are not
				v, found := strings.CutPrefix(p, prefix)
				return found && (strings.ContainsAny(v, " \t") || strings.HasPrefix(v, "-"))
			}
			if custom("input=") || custom("global=") {
				return fmt.Errorf("ffmpeg: sources may not override ffmpeg input/global arguments")
			}
			if v, found := strings.CutPrefix(p, "raw="); found {
				if err := checkRawArgs(v); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// normKind canonicalises a UI source kind for storage ("" and "raw" are the same
// verbatim power-user form as "advanced").
func normKind(kind string) string {
	switch kind {
	case "", "raw":
		return "advanced"
	}
	return kind
}

// rawFlags are the only ffmpeg output flags a raw= fragment may carry; anything
// else (-i, -f, -filter_complex, -protocol_whitelist, ...) could read or write
// local files.
var rawFlags = map[string]bool{
	"-vf": true, "-af": true, "-r": true, "-s": true, "-g": true, "-bf": true,
	"-b:v": true, "-b:a": true, "-c:v": true, "-c:a": true, "-q:v": true, "-q:a": true,
	"-an": true, "-vn": true, "-ar": true, "-ac": true, "-crf": true,
	"-maxrate": true, "-bufsize": true, "-preset": true, "-tune": true,
	"-profile:v": true, "-pix_fmt": true, "-rtsp_transport": true,
}

// checkRawArgs allow-lists the flags in an ffmpeg: raw= fragment and rejects any
// value that looks like a path, file/lavfi input or quoted argument.
func checkRawArgs(v string) error {
	for _, tok := range strings.Fields(v) {
		if strings.HasPrefix(tok, "-") && len(tok) > 1 && !(tok[1] >= '0' && tok[1] <= '9') {
			if !rawFlags[tok] {
				return fmt.Errorf("ffmpeg: raw= flag %q is not allowed", tok)
			}
			continue
		}
		if strings.ContainsAny(tok, "/\\\"'`") || strings.Contains(tok, "file:") ||
			strings.Contains(tok, "lavfi") || strings.Contains(tok, "movie") {
			return fmt.Errorf("ffmpeg: raw= value %q is not allowed", tok)
		}
	}
	return nil
}
