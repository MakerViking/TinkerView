package devices

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// idRe constrains printer ids: lowercase alnum start, then alnum/_/- up to 32
// chars. The id is used as a map key, bus event id, WS field, and REST path
// segment, so keep it conservative.
var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// camNameRe constrains camera (go2rtc/Frigate stream) names: lowercase alnum +
// underscore, matching the existing front_door / printer1 convention.
var camNameRe = regexp.MustCompile(`^[a-z0-9_]+$`)

// ValidatePrinter checks a printer record's shape and per-kind required fields.
// Uniqueness against the live set is enforced by the Manager (it needs runtime
// state); this is the stateless part shared by the WS and REST paths.
func ValidatePrinter(r PrinterRecord) error {
	if !idRe.MatchString(r.ID) {
		return fmt.Errorf("invalid id %q: use lowercase letters, digits, '-' or '_' (max 32)", r.ID)
	}
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("name is required")
	}
	switch r.Kind {
	case "moonraker":
		if err := validateHTTPURL("base_url", r.BaseURL); err != nil {
			return err
		}
		if r.UIURL != "" {
			if err := validateHTTPURL("ui_url", r.UIURL); err != nil {
				return err
			}
		}
	case "octoprint":
		if err := validateHTTPURL("base_url", r.BaseURL); err != nil {
			return err
		}
		if strings.TrimSpace(r.APIKey) == "" {
			return fmt.Errorf("api_key is required for octoprint")
		}
	case "prusalink":
		if err := validateHTTPURL("base_url", r.BaseURL); err != nil {
			return err
		}
		// api_key carries the PrusaLink password (Digest auth, hardcoded user "maker").
		if strings.TrimSpace(r.APIKey) == "" {
			return fmt.Errorf("api_key (PrusaLink password) is required for prusalink")
		}
	case "bambu":
		if strings.TrimSpace(r.IP) == "" || strings.TrimSpace(r.Serial) == "" || strings.TrimSpace(r.AccessCode) == "" {
			return fmt.Errorf("ip, serial and access_code are required for bambu")
		}
	default:
		return fmt.Errorf("unknown printer kind %q (expected moonraker, octoprint, prusalink or bambu)", r.Kind)
	}
	return nil
}

// ValidateCameraName checks a camera name's shape and the reserved _hq suffix.
// Collision checks (vs store / live go2rtc streams / Frigate config) live in the
// CameraManager, which has the runtime state to compare against.
func ValidateCameraName(name string) error {
	if !camNameRe.MatchString(name) {
		return fmt.Errorf("invalid camera name %q: use lowercase letters, digits and '_' only", name)
	}
	if strings.HasSuffix(name, "_hq") {
		return fmt.Errorf("camera names ending in %q are reserved for GPU-enhanced variants", "_hq")
	}
	return nil
}

func validateHTTPURL(field, raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("%s is required", field)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s is not a valid URL: %w", field, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s must start with http:// or https://", field)
	}
	if u.Host == "" {
		return fmt.Errorf("%s is missing a host", field)
	}
	return nil
}
