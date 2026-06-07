// Package automation watches the event bus and fires notifications on
// noteworthy transitions (print finished/failed, and Frigate camera alerts once
// that adapter exists). It also publishes an in-app Alert for printer events so
// they appear in the Alerts feed without a phone.
package automation

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/MakerViking/TinkerView/internal/adapter"
	"github.com/MakerViking/TinkerView/internal/bus"
	"github.com/MakerViking/TinkerView/internal/notify"
	"github.com/MakerViking/TinkerView/internal/rules"
)

const cooldown = 30 * time.Second

// alertRuler decides whether/how to push a Frigate detection. Implemented by
// *rules.Store; an interface so the engine can run without one (nil = legacy
// priority-by-kind behavior).
type alertRuler interface {
	Evaluate(rules.Input) rules.Decision
}

type Engine struct {
	bus      *bus.Bus
	notifier notify.Notifier
	rules    alertRuler

	mu        sync.Mutex
	lastState map[string]adapter.PState
	lastFired map[string]time.Time
	seq       int64
}

// New builds the engine. ruler (may be nil) applies the notification rule set to
// Frigate detections; without it, alerts use the static priority-by-kind fallback.
func New(b *bus.Bus, n notify.Notifier, ruler alertRuler) *Engine {
	return &Engine{
		bus: b, notifier: n, rules: ruler,
		lastState: map[string]adapter.PState{},
		lastFired: map[string]time.Time{},
	}
}

func (e *Engine) Run(ctx context.Context) {
	ch, cancel := e.bus.Subscribe("automation", 256)
	defer cancel()
	cleanup := time.NewTicker(time.Hour)
	defer cleanup.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-ch:
			e.handle(ctx, ev)
		case <-cleanup.C:
			e.pruneFired()
		}
	}
}

// pruneFired drops cooldown entries older than the cooldown window. Without this,
// per-event Frigate alert keys ("alert:<id>") would accumulate forever.
func (e *Engine) pruneFired() {
	e.mu.Lock()
	cutoff := time.Now().Add(-2 * cooldown)
	for k, t := range e.lastFired {
		if t.Before(cutoff) {
			delete(e.lastFired, k)
		}
	}
	e.mu.Unlock()
}

func (e *Engine) handle(ctx context.Context, ev bus.Event) {
	switch ev.Kind {
	case bus.KindPrinterState:
		ps, ok := ev.Payload.(adapter.PrinterState)
		if !ok {
			return
		}
		e.mu.Lock()
		prev, had := e.lastState[ps.ID]
		e.lastState[ps.ID] = ps.State
		e.mu.Unlock()
		if !had || prev == ps.State {
			return // no transition (or first sighting — don't fire on startup)
		}
		switch ps.State {
		case adapter.Complete:
			e.fire(ctx, "print_done:"+ps.ID,
				adapter.Alert{Kind: "print_done", PrinterID: ps.ID, Title: "Print finished — " + ps.Name, Message: ps.Filename},
				notify.Notification{Title: "✅ Print finished — " + ps.Name, Message: ps.Filename, Priority: 3, Tags: []string{"white_check_mark"}})
		case adapter.Errored, adapter.Cancelled:
			e.fire(ctx, "print_failed:"+ps.ID,
				adapter.Alert{Kind: "print_failed", PrinterID: ps.ID, Title: "Print " + string(ps.State) + " — " + ps.Name, Message: ps.Message},
				notify.Notification{Title: "❌ Print " + string(ps.State) + " — " + ps.Name, Message: ps.Message, Priority: 4, Tags: []string{"x"}})
		}

	case bus.KindAlert:
		// Frigate-origin alerts (person/motion/camera offline). Only NOTIFY — the
		// Frigate adapter already published the Alert for the in-app feed. Our own
		// system-origin alerts (printer events) are ignored here to avoid a loop.
		if ev.Source != bus.SourceFrigate {
			return
		}
		a, ok := ev.Payload.(adapter.Alert)
		if !ok {
			return
		}
		prio, suppressed := e.decide(a)
		if suppressed {
			slog.Debug("automation: alert suppressed by rule", "camera", a.Camera, "label", a.Label, "sub", a.SubLabel)
			return
		}
		e.fire(ctx, "alert:"+a.ID, adapter.Alert{},
			notify.Notification{Title: a.Title, Message: a.Message, Priority: prio, Tags: tagsFor(a.Kind)})
	}
}

// decide applies the notification rule set to a Frigate alert, returning the ntfy
// priority and whether the push is suppressed (ignored). With no rule engine
// configured it falls back to the static priority-by-kind policy.
func (e *Engine) decide(a adapter.Alert) (priority int, suppressed bool) {
	if e.rules == nil {
		return priorityFor(a.Kind), false
	}
	d := e.rules.Evaluate(rules.Input{
		Camera:   a.Camera,
		Label:    a.Label,
		SubLabel: a.SubLabel,
		Zones:    a.Zones,
		Now:      time.Now(),
	})
	return d.Priority, !d.Notify
}

// fire enforces a per-key cooldown, optionally publishes an in-app Alert (when
// alert.Kind is set), and sends the push notification.
func (e *Engine) fire(ctx context.Context, key string, alert adapter.Alert, n notify.Notification) {
	e.mu.Lock()
	if t, ok := e.lastFired[key]; ok && time.Since(t) < cooldown {
		e.mu.Unlock()
		return
	}
	e.lastFired[key] = time.Now()
	e.seq++
	seq := e.seq
	e.mu.Unlock()

	if alert.Kind != "" {
		alert.ID = fmt.Sprintf("%s-%d", alert.Kind, seq)
		alert.Ts = time.Now().UnixMilli()
		e.bus.Publish(bus.Event{Source: bus.SourceSystem, Kind: bus.KindAlert, ID: alert.ID, Ts: time.Now(), Payload: alert})
	}
	if err := e.notifier.Send(ctx, n); err != nil {
		slog.Warn("automation: notification failed", "key", key, "err", err)
	}
}

func priorityFor(kind string) int {
	switch kind {
	case "person":
		return 4
	case "camera_offline", "frigate_down":
		return 4
	default:
		return 3
	}
}

func tagsFor(kind string) []string {
	switch kind {
	case "person":
		return []string{"rotating_light"}
	case "motion":
		return []string{"cyclone"}
	case "camera_offline":
		return []string{"no_entry"}
	case "frigate_down":
		return []string{"warning"}
	default:
		return nil
	}
}
