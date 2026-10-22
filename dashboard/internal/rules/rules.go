// Package rules is the notification-priority policy for Frigate detections. Each
// incoming alert (camera + object label + recognized name + zones + time of day)
// is matched against an ordered list of rules; the first match decides whether to
// push a phone notification, push it silently, or ignore it — and at what ntfy
// priority. Rules are persisted as JSON under the writable data dir and editable
// from the dashboard's Recognition tab.
//
// The known/unknown distinction is derived purely from the recognized SubLabel: a
// non-empty SubLabel means a face or plate was recognized ("known"); empty means
// "unknown". A rule can also match a specific recognized name.
package rules

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Action is what to do when a rule matches.
const (
	ActionPush   = "push"   // send a phone push at the rule's Priority
	ActionSilent = "silent" // send, but at min priority (no sound/vibration)
	ActionIgnore = "ignore" // no push at all (the in-app feed still shows it)
)

// Match modes for the recognized SubLabel.
const (
	MatchAny     = "any"     // or "" — any object
	MatchKnown   = "known"   // a face/plate was recognized (SubLabel present)
	MatchUnknown = "unknown" // not recognized (SubLabel empty)
	// any other value is treated as a specific recognized name (case-insensitive).
)

// Rule is one ordered policy entry. Empty selector fields mean "any". Evaluation
// stops at the first rule that matches.
type Rule struct {
	Name     string `json:"name,omitempty"`     // human label for the UI
	Camera   string `json:"camera,omitempty"`   // "" = any camera
	Label    string `json:"label,omitempty"`    // object: person|car|dog|… ; "" = any
	Match    string `json:"match,omitempty"`    // any|known|unknown|<recognized-name>
	Zone     string `json:"zone,omitempty"`     // "" = any; matches if the alert entered this zone
	From     string `json:"from,omitempty"`     // local "HH:MM" window start (inclusive)
	To       string `json:"to,omitempty"`       // local "HH:MM" window end (exclusive); From>To = overnight
	Action   string `json:"action"`             // push|silent|ignore
	Priority int    `json:"priority,omitempty"` // 1..5 ntfy priority for push (default 3)
}

// Input is a normalized detection to evaluate.
type Input struct {
	Camera   string
	Label    string
	SubLabel string
	Zones    []string
	Now      time.Time
}

// Decision is the outcome of evaluating Input against the rule set.
type Decision struct {
	Notify   bool   // send a push?
	Priority int    // ntfy priority (1..5) when Notify
	Rule     string // matched rule name (for logging); "" = default fallthrough
}

// DefaultRules is the seeded policy used when no rules file exists yet. Recognized
// faces/plates are silent; an unknown person at night is loud; people and cars are
// normal pushes. Anything unmatched falls through to a default priority-3 push.
func DefaultRules() []Rule {
	return []Rule{
		{Name: "Recognized face/plate — silent", Match: MatchKnown, Action: ActionSilent},
		{Name: "Unknown person at night", Label: "person", Match: MatchUnknown, From: "21:00", To: "06:00", Action: ActionPush, Priority: 5},
		{Name: "Person", Label: "person", Action: ActionPush, Priority: 4},
		{Name: "Car", Label: "car", Action: ActionPush, Priority: 3},
	}
}

// Store holds the rule set and persists it atomically. Safe for concurrent use:
// the automation engine reads (Evaluate) while the API writes (Set).
type Store struct {
	mu    sync.RWMutex
	path  string
	rules []Rule
}

// Open loads rules from path. A missing file seeds DefaultRules and writes them so
// the file exists for the user to inspect/edit. A malformed file is a hard error.
func Open(path string) (*Store, error) {
	s := &Store{path: path}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		s.rules = DefaultRules()
		// An unwritable data dir must not be fatal (the hub shows a banner and keeps
		// running): serve the in-memory defaults; Set reports the error when edited.
		if err := s.persist(s.rules); err != nil {
			slog.Warn("notification rules: could not seed default file; using in-memory defaults", "path", path, "err", err)
		}
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var rs []Rule
	if err := json.Unmarshal(b, &rs); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	s.rules = rs
	return s, nil
}

// Get returns a copy of the current rules.
func (s *Store) Get() []Rule {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Rule(nil), s.rules...)
}

// Set validates, persists, and swaps in a new rule set.
func (s *Store) Set(rs []Rule) error {
	clean := make([]Rule, 0, len(rs))
	for i, r := range rs {
		nr, err := validate(r)
		if err != nil {
			return fmt.Errorf("rule %d: %w", i+1, err)
		}
		clean = append(clean, nr)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.persist(clean); err != nil {
		return err
	}
	s.rules = clean
	return nil
}

// Evaluate returns the decision for an input: the first matching rule wins; with
// no match it defaults to a normal (priority-3) push so a new object type is never
// silently dropped.
func (s *Store) Evaluate(in Input) Decision {
	s.mu.RLock()
	rules := s.rules
	s.mu.RUnlock()
	for _, r := range rules {
		if r.matches(in) {
			return decide(r)
		}
	}
	return Decision{Notify: true, Priority: 3}
}

func decide(r Rule) Decision {
	switch r.Action {
	case ActionIgnore:
		return Decision{Notify: false, Rule: r.Name}
	case ActionSilent:
		return Decision{Notify: true, Priority: 1, Rule: r.Name}
	default: // push
		p := r.Priority
		if p < 1 || p > 5 {
			p = 3
		}
		return Decision{Notify: true, Priority: p, Rule: r.Name}
	}
}

func (r Rule) matches(in Input) bool {
	if r.Camera != "" && !strings.EqualFold(r.Camera, in.Camera) {
		return false
	}
	if r.Label != "" && !strings.EqualFold(r.Label, in.Label) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(r.Match)) {
	case "", MatchAny:
		// any
	case MatchKnown:
		if in.SubLabel == "" {
			return false
		}
	case MatchUnknown:
		if in.SubLabel != "" {
			return false
		}
	default: // a specific recognized name
		if !strings.EqualFold(strings.TrimSpace(r.Match), in.SubLabel) {
			return false
		}
	}
	if r.Zone != "" && !containsFold(in.Zones, r.Zone) {
		return false
	}
	return inWindow(r.From, r.To, in.Now)
}

func containsFold(list []string, want string) bool {
	for _, s := range list {
		if strings.EqualFold(s, want) {
			return true
		}
	}
	return false
}

// inWindow reports whether t's local hour:minute falls within [from, to). Empty
// from and to mean "always". from > to denotes an overnight window (e.g. 21:00 →
// 06:00 spans midnight).
func inWindow(from, to string, t time.Time) bool {
	if from == "" && to == "" {
		return true
	}
	f, okF := parseHM(from)
	o, okT := parseHM(to)
	if !okF || !okT {
		return true // malformed window never blocks (validate rejects these on Set)
	}
	now := t.Hour()*60 + t.Minute()
	if f == o {
		return true // zero-width window treated as all-day
	}
	if f < o {
		return now >= f && now < o
	}
	return now >= f || now < o // overnight
}

// parseHM parses "HH:MM" into minutes-of-day.
func parseHM(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	var h, m int
	if _, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil {
		return 0, false
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

func validate(r Rule) (Rule, error) {
	r.Action = strings.ToLower(strings.TrimSpace(r.Action))
	switch r.Action {
	case ActionPush, ActionSilent, ActionIgnore:
	case "":
		r.Action = ActionPush
	default:
		return r, fmt.Errorf("unknown action %q (want push|silent|ignore)", r.Action)
	}
	if r.Priority != 0 && (r.Priority < 1 || r.Priority > 5) {
		return r, fmt.Errorf("priority must be 1..5, got %d", r.Priority)
	}
	if r.From != "" || r.To != "" {
		if _, ok := parseHM(r.From); !ok {
			return r, fmt.Errorf("invalid 'from' time %q (want HH:MM)", r.From)
		}
		if _, ok := parseHM(r.To); !ok {
			return r, fmt.Errorf("invalid 'to' time %q (want HH:MM)", r.To)
		}
	}
	return r, nil
}

// persist atomically writes rs to the rules file (temp + rename on the same dir).
func (s *Store) persist(rs []Rule) error {
	b, err := json.MarshalIndent(rs, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, ".notify-rules-*.json")
	if err != nil {
		return fmt.Errorf("create temp in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
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
