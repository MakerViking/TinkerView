package devices

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/MakerViking/TinkerView/internal/adapter"
	"github.com/MakerViking/TinkerView/internal/adapters/bambu"
	"github.com/MakerViking/TinkerView/internal/adapters/moonraker"
	"github.com/MakerViking/TinkerView/internal/adapters/octoprint"
	"github.com/MakerViking/TinkerView/internal/adapters/prusalink"
	"github.com/MakerViking/TinkerView/internal/bus"
)

// RunFn launches fn on a supervised goroutine (main supplies its WaitGroup +
// panic-recovery wrapper). The Manager ignores the ctx passed to fn and uses a
// per-printer child ctx instead, so each adapter can be cancelled independently
// while still draining on shutdown.
type RunFn func(name string, fn func(context.Context))

// Manager owns the live printer adapters and their per-printer cancel funcs, and
// persists UI changes to the Store. It is the single writer of the live maps.
type Manager struct {
	bus       *bus.Bus
	store     *Store
	run       RunFn
	parentCtx context.Context

	mu       sync.Mutex
	printers map[string]adapter.Printer
	cancels  map[string]context.CancelFunc
	// done[id] is closed when printer id's adapter goroutine has returned, so an
	// edit can wait for the old adapter before starting its replacement.
	done map[string]chan struct{}
}

// adapterStopWait bounds how long EditPrinter waits for a cancelled adapter to
// exit before starting its replacement (an adapter stuck in blocking I/O must
// not wedge the edit).
const adapterStopWait = 2 * time.Second

// NewManager builds a Manager. parentCtx is the process root context; every
// printer goroutine derives a child from it, so the existing stop()/wg.Wait()
// shutdown drains runtime-added printers too.
func NewManager(parentCtx context.Context, b *bus.Bus, store *Store, run RunFn) *Manager {
	m := &Manager{
		bus: b, store: store, run: run, parentCtx: parentCtx,
		printers: map[string]adapter.Printer{},
		cancels:  map[string]context.CancelFunc{},
		done:     map[string]chan struct{}{},
	}
	// A camera rename/delete rewrites printers' camera reference in the store;
	// reload the affected live adapters so they pick up the new name.
	store.SetPrinterRefHook(m.reloadPrinters)
	return m
}

// PrinterRecord returns the stored (unredacted) record for id. Callers that
// serve it to clients must use PrinterRecord.View().
func (m *Manager) PrinterRecord(id string) (PrinterRecord, bool) {
	return m.store.Printer(id)
}

// reloadPrinters restarts the live adapters for ids from their (updated) store
// records. Called by the store after a camera reference changes.
func (m *Manager) reloadPrinters(ids []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		if _, running := m.printers[id]; !running {
			continue
		}
		rec, ok := m.store.Printer(id)
		if !ok {
			continue
		}
		m.waitStopped(m.stopLocked(id))
		if err := m.startLocked(rec); err != nil {
			slog.Warn("printer reload after camera change failed", "id", id, "err", err)
		}
	}
}

// RestorePrinter starts a printer that is already persisted (startup path). It
// skips re-persisting and tolerates an already-known id (logs + skips).
func (m *Manager) RestorePrinter(r PrinterRecord) error {
	if err := ValidatePrinter(r); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.printers[r.ID]; exists {
		return fmt.Errorf("printer %q already running", r.ID)
	}
	return m.startLocked(r)
}

// AddPrinter validates, persists, then starts a new printer. Persist-before-start
// means a save failure aborts cleanly with no orphaned goroutine.
func (m *Manager) AddPrinter(r PrinterRecord) error {
	if err := ValidatePrinter(r); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.printers[r.ID]; exists {
		return fmt.Errorf("a printer with id %q already exists", r.ID)
	}
	if err := m.store.UpsertPrinter(r); err != nil {
		return fmt.Errorf("save printer: %w", err)
	}
	return m.startLocked(r)
}

// EditPrinter re-initialises an existing printer in place (id and kind are
// immutable). Blank connection/secret fields keep their stored value, so an edit
// form need not re-send (or expose) the API key / Bambu credentials to change
// just a name; a blank ui_url or camera clears it. The whole read-modify-write
// runs under the lock, and the old adapter goroutine is awaited (bounded) before
// its replacement starts so a stale adapter can't publish over the new state.
func (m *Manager) EditPrinter(id string, r PrinterRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.printers[id]; !ok {
		return fmt.Errorf("unknown printer %q", id)
	}
	existing, ok := m.store.Printer(id)
	if !ok {
		return fmt.Errorf("unknown printer %q", id)
	}
	r.ID = id
	if r.Kind == "" {
		r.Kind = existing.Kind
	}
	if r.Kind != existing.Kind {
		return fmt.Errorf("a printer's kind can't be changed (%s -> %s); remove it and add a new one", existing.Kind, r.Kind)
	}
	if r.BaseURL == "" {
		r.BaseURL = existing.BaseURL
	}
	if r.APIKey == "" {
		r.APIKey = existing.APIKey
	}
	if r.IP == "" {
		r.IP = existing.IP
	}
	if r.Serial == "" {
		r.Serial = existing.Serial
	}
	if r.AccessCode == "" {
		r.AccessCode = existing.AccessCode
	}
	if r.Model == "" {
		r.Model = existing.Model
	}
	if r.Macros == nil {
		r.Macros = existing.Macros
	}
	if err := ValidatePrinter(r); err != nil {
		return err
	}
	if err := m.store.UpsertPrinter(r); err != nil {
		return fmt.Errorf("save printer: %w", err)
	}
	m.waitStopped(m.stopLocked(id)) // old adapter goroutine exits on ctx cancel
	return m.startLocked(r)         // start fresh with the new config
}

// RemovePrinter stops the adapter, removes it from the store, and tells clients
// to drop the tile. Persist-before-stop means a save failure leaves it running.
func (m *Manager) RemovePrinter(id string) error {
	m.mu.Lock()
	if _, ok := m.printers[id]; !ok {
		m.mu.Unlock()
		return fmt.Errorf("unknown printer %q", id)
	}
	if _, err := m.store.DeletePrinter(id); err != nil {
		m.mu.Unlock()
		return fmt.Errorf("save printer store: %w", err)
	}
	m.stopLocked(id)
	m.mu.Unlock()
	// Publish the delete outside the lock (keeps "no external calls under m.mu").
	m.bus.Publish(bus.Event{
		Source: bus.SourcePrinter, Kind: bus.KindPrinterState, ID: id, Ts: time.Now(),
		Payload: adapter.PrinterState{ID: id, Deleted: true},
	})
	return nil
}

// AddRawAdapter registers an already-built adapter (used for the demo printer).
// It is not persisted. Caller must not hold m.mu.
func (m *Manager) AddRawAdapter(a adapter.Printer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := a.ID()
	if _, exists := m.printers[id]; exists {
		return
	}
	cctx, cancel := context.WithCancel(m.parentCtx)
	done := make(chan struct{})
	m.printers[id] = a
	m.cancels[id] = cancel
	m.done[id] = done
	m.run("printer:"+id, func(context.Context) {
		defer close(done)
		a.Run(cctx)
	})
}

// startLocked builds the adapter for r and launches it on a per-printer child
// ctx. Caller holds m.mu. Unknown kinds are rejected (validation catches them first).
func (m *Manager) startLocked(r PrinterRecord) error {
	if m.parentCtx.Err() != nil {
		return nil // shutting down; don't start new goroutines after wg.Wait
	}
	switch r.Kind {
	case "moonraker":
		return m.launchLocked(r, moonraker.New(m.bus, r.ID, r.Name, r.BaseURL, r.APIKey, r.Camera, r.UIURL, true, r.Macros))
	case "octoprint":
		return m.launchLocked(r, octoprint.New(m.bus, r.ID, r.Name, r.BaseURL, r.APIKey, r.Camera, r.UIURL, true, r.Macros))
	case "prusalink":
		return m.launchLocked(r, prusalink.New(m.bus, r.ID, r.Name, r.BaseURL, r.APIKey, r.Camera, r.UIURL, true, r.Macros))
	case "bambu":
		return m.launchLocked(r, bambu.New(m.bus, r.ID, r.Name, r.IP, r.Serial, r.AccessCode, r.Model, r.Camera, r.UIURL, true, r.Macros))
	default:
		return fmt.Errorf("unknown printer kind %q", r.Kind)
	}
}

// launchLocked registers an already-built adapter under a fresh per-printer child
// ctx and starts its supervised goroutine. Caller holds m.mu.
func (m *Manager) launchLocked(r PrinterRecord, p adapter.Printer) error {
	cctx, cancel := context.WithCancel(m.parentCtx)
	done := make(chan struct{})
	m.printers[r.ID] = p
	m.cancels[r.ID] = cancel
	m.done[r.ID] = done
	m.run("printer:"+r.ID, func(context.Context) {
		defer close(done)
		p.Run(cctx)
	})
	slog.Info("printer added", "id", r.ID, "kind", r.Kind)
	return nil
}

// stopLocked cancels a printer's ctx (its goroutine exits) and drops it from the
// live maps. It returns the channel closed when that goroutine has returned (nil
// if the printer wasn't running). Caller holds m.mu.
func (m *Manager) stopLocked(id string) <-chan struct{} {
	if cancel, ok := m.cancels[id]; ok {
		cancel()
		delete(m.cancels, id)
	}
	delete(m.printers, id)
	done := m.done[id]
	delete(m.done, id)
	return done
}

// waitStopped waits (bounded by adapterStopWait) for a stopped adapter's
// goroutine to return. A nil channel is a no-op.
func (m *Manager) waitStopped(done <-chan struct{}) {
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(adapterStopWait):
		slog.Warn("printer adapter did not stop within the wait; starting replacement anyway")
	}
}

// ----- hub.CommandHandler (printer portion) -----

func (m *Manager) PrinterCommand(ctx context.Context, id string, cmd adapter.PrinterCommand) error {
	m.mu.Lock()
	p, ok := m.printers[id]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("unknown printer %q", id)
	}
	return p.Command(ctx, cmd) // network I/O outside the lock
}

func (m *Manager) PrinterSetTemp(ctx context.Context, id, heater string, target float64) error {
	m.mu.Lock()
	p, ok := m.printers[id]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("unknown printer %q", id)
	}
	return p.SetTemp(ctx, heater, target)
}

func (m *Manager) PrinterControl(ctx context.Context, id string, a adapter.PrinterAction) error {
	m.mu.Lock()
	p, ok := m.printers[id]
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("unknown printer %q", id)
	}
	return p.Control(ctx, a)
}

// PrinterTarget returns a printer's Moonraker base URL + API key for the read-only
// browse proxy. Reads the store (works even while an adapter is reconnecting).
// Implements web.TargetResolver.
func (m *Manager) PrinterTarget(id string) (baseURL, apiKey string, ok bool) {
	r, found := m.store.Printer(id)
	if !found || r.Kind != "moonraker" {
		return "", "", false
	}
	return r.BaseURL, r.APIKey, true
}
