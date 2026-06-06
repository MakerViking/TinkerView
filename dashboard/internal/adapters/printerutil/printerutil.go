// Package printerutil holds the small, pure helpers shared by the printer
// adapters (moonraker, octoprint, bambu): numeric clamps, the home-gcode builder,
// the macro allowlist guard, and the shared safety constants. Keeping these in one
// place means a change to the motion envelope or the injection guard lands once.
package printerutil

import (
	"errors"
	"strings"
)

// DefaultMinExtrudeTemp is the cold-extrusion floor (°C), mirroring Klipper/Marlin's
// default so a cold extrude is refused before the gcode is ever sent.
const DefaultMinExtrudeTemp = 170

// ErrNotIdle is returned by an adapter when an action requires the printer to be idle.
var ErrNotIdle = errors.New("printer must be idle for this action")

// Clamp constrains v to [lo, hi].
func Clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ClampAbs constrains v to [-max, max] (signed distance limiter).
func ClampAbs(v, max float64) float64 {
	if v > max {
		return max
	}
	if v < -max {
		return -max
	}
	return v
}

// ClampFeed picks a per-axis default feedrate when feed<=0 and clamps to that axis's
// envelope: xy 60..6000, z 60..1200, e 60..600 mm/min.
func ClampFeed(feed float64, axis string) float64 {
	def, max := 3000.0, 6000.0
	switch strings.ToLower(axis) {
	case "z":
		def, max = 600, 1200
	case "e":
		def, max = 300, 600
	}
	if feed <= 0 {
		feed = def
	}
	return Clamp(feed, 60, max)
}

// HomeGcode builds a G28 home command for the given axes ("" homes all; "xz" -> "G28 X Z").
func HomeGcode(axes string) string {
	axes = strings.ToLower(strings.TrimSpace(axes))
	if axes == "" {
		return "G28"
	}
	var b strings.Builder
	b.WriteString("G28")
	for _, c := range axes {
		if c == 'x' || c == 'y' || c == 'z' {
			b.WriteByte(' ')
			b.WriteRune(c - 32) // to upper
		}
	}
	return b.String()
}

// MacroAllowed reports whether name is an exact, case-insensitive member of allowed
// and carries no injection characters (leading/trailing or embedded whitespace, or a
// semicolon) that could smuggle a second gcode command.
func MacroAllowed(name string, allowed []string) bool {
	if name == "" || strings.TrimSpace(name) != name || strings.ContainsAny(name, " \t\n\r;") {
		return false
	}
	for _, m := range allowed {
		if strings.EqualFold(m, name) {
			return true
		}
	}
	return false
}
