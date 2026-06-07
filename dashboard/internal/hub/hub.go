package hub

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/MakerViking/TinkerView/internal/adapter"
	"github.com/MakerViking/TinkerView/internal/bus"
	"github.com/MakerViking/TinkerView/internal/devices"
)

const alertRing = 100

// printerTombstoneTTL is how long a just-deleted printer id is ignored, so an
// in-flight stale printer.state from a cancelled adapter can't resurrect the tile.
const printerTombstoneTTL = 5 * time.Second

// CommandHandler routes inbound client commands to the owning adapter/manager.
// main wires an implementation backed by the printer/camera/docker registries.
type CommandHandler interface {
	PrinterCommand(ctx context.Context, id string, cmd adapter.PrinterCommand) error
	PrinterSetTemp(ctx context.Context, id, heater string, target float64) error
	PrinterControl(ctx context.Context, id string, a adapter.PrinterAction) error
	CameraToggle(ctx context.Context, camera string, feature adapter.CameraFeature, value, persist bool) error
	ContainerRestart(ctx context.Context, id string) error

	// Device CRUD. Errors are surfaced to the user as a notice / non-2xx REST
	// status. Camera promote/demote (and remove-of-promoted) may run asynchronously
	// and report progress via notices; they return quickly.
	AddPrinter(r devices.PrinterRecord) error
	EditPrinter(id string, r devices.PrinterRecord) error
	RemovePrinter(id string) error
	AddCamera(name, srcKind, srcInput, username, password string) error
	EditCamera(name, newName, newSrcKind, newSrcInput, username, password string) error
	RemoveCamera(name string) error
	PromoteCamera(name string, detect, record bool, recordDays int, snapshots bool) error
	DemoteCamera(name string) error
	EditCameraSettings(name string, s devices.CameraSettings) error
	CameraSettings(name string) (devices.CameraSettings, bool)
}

// Hub fans bus events out to every connected WebSocket client and routes inbound
// commands back to adapters. It keeps an authoritative cache so a newly-connected
// client gets a full snapshot immediately.
type Hub struct {
	bus   *bus.Bus
	cmd   CommandHandler
	token string

	mu                sync.RWMutex
	clients           map[*client]struct{}
	cameras           map[string]adapter.CameraState
	printers          map[string]adapter.PrinterState
	printerTombstones map[string]time.Time // recently-deleted ids -> deadline
	containers        []adapter.Container
	disks             []adapter.DiskUsage
	frigateOnline     bool
	alerts            []adapter.Alert
}

// New builds a Hub. token (optional) gates the WS handshake via ?token=.
func New(b *bus.Bus, cmd CommandHandler, token string) *Hub {
	return &Hub{
		bus: b, cmd: cmd, token: token,
		clients:           make(map[*client]struct{}),
		cameras:           make(map[string]adapter.CameraState),
		printers:          make(map[string]adapter.PrinterState),
		printerTombstones: make(map[string]time.Time),
	}
}

// Run consumes the bus, maintains the cache, and broadcasts deltas. Blocks until
// ctx is cancelled.
func (h *Hub) Run(ctx context.Context) {
	ch, cancel := h.bus.Subscribe("hub", 512)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-ch:
			h.ingest(e)
		}
	}
}

func (h *Hub) ingest(e bus.Event) {
	var msg ServerMsg
	h.mu.Lock()
	switch e.Kind {
	case bus.KindCameraState:
		cs, ok := e.Payload.(adapter.CameraState)
		if !ok {
			h.mu.Unlock()
			typeMismatch(e.Kind)
			return
		}
		if cs.Deleted {
			delete(h.cameras, cs.Name)
			msg = ServerMsg{Type: "camera.delete", Camera: cs.Name}
		} else {
			cur := h.cameras[cs.Name]
			cur.Name = cs.Name
			mergeCamera(&cur, cs)
			h.cameras[cs.Name] = cur
			msg = ServerMsg{Type: "camera.state", Camera: cs.Name, Data: cs}
		}
	case bus.KindCameraStatus:
		cs, ok := e.Payload.(CameraStatus)
		if !ok {
			h.mu.Unlock()
			typeMismatch(e.Kind)
			return
		}
		msg = ServerMsg{Type: "camera.status", Camera: e.ID, Role: cs.Role, Value: cs.Value}
	case bus.KindPrinterState:
		ps, ok := e.Payload.(adapter.PrinterState)
		if !ok {
			h.mu.Unlock()
			typeMismatch(e.Kind)
			return
		}
		if ps.Deleted {
			delete(h.printers, ps.ID)
			h.printerTombstones[ps.ID] = time.Now().Add(printerTombstoneTTL)
			msg = ServerMsg{Type: "printer.delete", Data: map[string]string{"id": ps.ID}}
		} else {
			// Ignore a stale state for a just-deleted printer (a cancelled adapter's
			// last in-flight publish), so a removed tile can't resurrect.
			if dl, ok := h.printerTombstones[ps.ID]; ok {
				if time.Now().Before(dl) {
					h.mu.Unlock()
					return
				}
				delete(h.printerTombstones, ps.ID)
			}
			h.printers[ps.ID] = ps
			msg = ServerMsg{Type: "printer.state", Data: ps}
		}
	case bus.KindContainers:
		cs, ok := e.Payload.([]adapter.Container)
		if !ok {
			h.mu.Unlock()
			typeMismatch(e.Kind)
			return
		}
		h.containers = cs
		msg = ServerMsg{Type: "containers", Data: h.containers}
	case bus.KindDisks:
		ds, ok := e.Payload.([]adapter.DiskUsage)
		if !ok {
			h.mu.Unlock()
			typeMismatch(e.Kind)
			return
		}
		h.disks = ds
		msg = ServerMsg{Type: "disks", Data: h.disks}
	case bus.KindFrigate:
		up, ok := e.Payload.(bool)
		if !ok {
			h.mu.Unlock()
			typeMismatch(e.Kind)
			return
		}
		h.frigateOnline = up
		msg = ServerMsg{Type: "frigate", Data: map[string]bool{"online": up}}
	case bus.KindAlert:
		a, ok := e.Payload.(adapter.Alert)
		if !ok {
			h.mu.Unlock()
			typeMismatch(e.Kind)
			return
		}
		h.alerts = append(h.alerts, a)
		if len(h.alerts) > alertRing {
			h.alerts = h.alerts[len(h.alerts)-alertRing:]
		}
		msg = ServerMsg{Type: "alert", Data: a}
	case bus.KindNotice:
		n, ok := e.Payload.(Notice)
		if !ok {
			h.mu.Unlock()
			typeMismatch(e.Kind)
			return
		}
		msg = ServerMsg{Type: "notice", Level: n.Level, Msg: n.Msg}
	default:
		h.mu.Unlock()
		return
	}
	h.mu.Unlock()
	h.broadcast(msg)
}

func typeMismatch(kind bus.Kind) {
	slog.Warn("hub: event payload type mismatch", "kind", kind)
}

// CameraStatus is the bus payload for KindCameraStatus.
type CameraStatus struct {
	Role  string
	Value string
}

// ServeWS upgrades the request and serves one client until it disconnects.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	if h.token != "" && r.URL.Query().Get("token") != h.token {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Reached over the tailnet from the dashboard origin; same-origin check
		// is unnecessary and would block the SPA served from a different port in dev.
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	c := &client{hub: h, conn: conn, send: make(chan []byte, 64), done: make(chan struct{})}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()

	c.send <- h.snapshotBytes() // initial full state

	ctx := r.Context()
	go c.writeLoop(ctx)
	c.readLoop(ctx) // blocks until the conn closes
	h.removeClient(c)
}

// Snapshot returns a deep-enough copy of the current cached state (for the WS
// snapshot message and GET /api/state).
func (h *Hub) Snapshot() Snapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return Snapshot{
		Cameras:       maps.Clone(h.cameras),
		Printers:      maps.Clone(h.printers),
		Containers:    h.containers,
		Disks:         h.disks,
		FrigateOnline: h.frigateOnline,
		Alerts:        append([]adapter.Alert(nil), h.alerts...),
	}
}

func (h *Hub) snapshotBytes() []byte {
	data, _ := json.Marshal(ServerMsg{Type: "snapshot", V: 1, Data: h.Snapshot()})
	return data
}

func (h *Hub) broadcast(msg ServerMsg) {
	data, err := json.Marshal(msg)
	if err != nil {
		slog.Error("hub: marshal server msg", "err", err)
		return
	}
	var slow []*client
	h.mu.RLock()
	for c := range h.clients {
		select {
		case c.send <- data:
		default:
			slow = append(slow, c)
		}
	}
	h.mu.RUnlock()
	for _, c := range slow {
		slog.Warn("hub: dropping slow client")
		h.removeClient(c)
	}
}

func (h *Hub) removeClient(c *client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.done) // signal readLoop to stop sending before we close send
		close(c.send)
	}
	h.mu.Unlock()
}

func (h *Hub) handleCommand(ctx context.Context, raw []byte) {
	var m ClientMsg
	if err := json.Unmarshal(raw, &m); err != nil {
		return
	}
	if h.cmd == nil {
		return
	}
	switch m.Type {
	case "camera.toggle":
		var d CameraToggleCmd
		if err := json.Unmarshal(m.Data, &d); err != nil {
			h.notice("warn", "malformed "+m.Type+" command: "+err.Error())
			return
		}
		if err := h.cmd.CameraToggle(ctx, d.Camera, adapter.CameraFeature(d.Feature), d.Value, d.Persist); err != nil {
			h.notice("warn", err.Error())
		}
	case "printer.cmd":
		var d PrinterCmd
		if err := json.Unmarshal(m.Data, &d); err != nil {
			h.notice("warn", "malformed "+m.Type+" command: "+err.Error())
			return
		}
		if err := h.cmd.PrinterCommand(ctx, d.ID, adapter.PrinterCommand(d.Cmd)); err != nil {
			h.notice("warn", err.Error())
		}
	case "printer.temp":
		var d PrinterTempCmd
		if err := json.Unmarshal(m.Data, &d); err != nil {
			h.notice("warn", "malformed "+m.Type+" command: "+err.Error())
			return
		}
		if err := h.cmd.PrinterSetTemp(ctx, d.ID, d.Heater, d.Target); err != nil {
			h.notice("warn", err.Error())
		}
	case "printer.control":
		var d PrinterControlCmd
		if err := json.Unmarshal(m.Data, &d); err != nil {
			h.notice("warn", "malformed "+m.Type+" command: "+err.Error())
			return
		}
		if err := h.cmd.PrinterControl(ctx, d.ID, d.Action); err != nil {
			h.notice("warn", err.Error())
		}
	case "container.restart":
		var d ContainerRestartCmd
		if err := json.Unmarshal(m.Data, &d); err != nil {
			h.notice("warn", "malformed "+m.Type+" command: "+err.Error())
			return
		}
		if err := h.cmd.ContainerRestart(ctx, d.ID); err != nil {
			h.notice("warn", err.Error())
		}
	case "printer.add":
		var d PrinterAddCmd
		if err := json.Unmarshal(m.Data, &d); err != nil {
			h.notice("warn", "malformed "+m.Type+" command: "+err.Error())
			return
		}
		if err := h.cmd.AddPrinter(d.Printer); err != nil {
			h.notice("warn", err.Error())
		}
	case "printer.edit":
		var d PrinterEditCmd
		if err := json.Unmarshal(m.Data, &d); err != nil {
			h.notice("warn", "malformed "+m.Type+" command: "+err.Error())
			return
		}
		if err := h.cmd.EditPrinter(d.ID, d.Printer); err != nil {
			h.notice("warn", err.Error())
		}
	case "printer.remove":
		var d PrinterRemoveCmd
		if err := json.Unmarshal(m.Data, &d); err != nil {
			h.notice("warn", "malformed "+m.Type+" command: "+err.Error())
			return
		}
		if err := h.cmd.RemovePrinter(d.ID); err != nil {
			h.notice("warn", err.Error())
		}
	case "camera.add":
		var d CameraAddCmd
		if err := json.Unmarshal(m.Data, &d); err != nil {
			h.notice("warn", "malformed "+m.Type+" command: "+err.Error())
			return
		}
		if err := h.cmd.AddCamera(d.Name, d.SrcKind, d.SrcInput, d.Username, d.Password); err != nil {
			h.notice("warn", err.Error())
		}
	case "camera.edit":
		var d CameraEditCmd
		if err := json.Unmarshal(m.Data, &d); err != nil {
			h.notice("warn", "malformed "+m.Type+" command: "+err.Error())
			return
		}
		if err := h.cmd.EditCamera(d.Name, d.NewName, d.NewSrcKind, d.NewSrcInput, d.Username, d.Password); err != nil {
			h.notice("warn", err.Error())
		}
	case "camera.remove":
		var d CameraRemoveCmd
		if err := json.Unmarshal(m.Data, &d); err != nil {
			h.notice("warn", "malformed "+m.Type+" command: "+err.Error())
			return
		}
		if err := h.cmd.RemoveCamera(d.Name); err != nil {
			h.notice("warn", err.Error())
		}
	case "camera.promote":
		var d CameraPromoteCmd
		if err := json.Unmarshal(m.Data, &d); err != nil {
			h.notice("warn", "malformed "+m.Type+" command: "+err.Error())
			return
		}
		if err := h.cmd.PromoteCamera(d.Name, d.Detect, d.Record, d.RecordDays, d.Snapshots); err != nil {
			h.notice("warn", err.Error())
		}
	case "camera.demote":
		var d CameraDemoteCmd
		if err := json.Unmarshal(m.Data, &d); err != nil {
			h.notice("warn", "malformed "+m.Type+" command: "+err.Error())
			return
		}
		if err := h.cmd.DemoteCamera(d.Name); err != nil {
			h.notice("warn", err.Error())
		}
	case "camera.settings":
		var d CameraSettingsCmd
		if err := json.Unmarshal(m.Data, &d); err != nil {
			h.notice("warn", "malformed "+m.Type+" command: "+err.Error())
			return
		}
		if err := h.cmd.EditCameraSettings(d.Name, d.Settings); err != nil {
			h.notice("warn", err.Error())
		}
	case "ping":
		// liveness only
	}
}

func (h *Hub) notice(level, msg string) {
	h.broadcast(ServerMsg{Type: "notice", Level: level, Msg: msg})
}

func mergeCamera(cur *adapter.CameraState, d adapter.CameraState) {
	if d.Detect != nil {
		cur.Detect = d.Detect
	}
	if d.Recordings != nil {
		cur.Recordings = d.Recordings
	}
	if d.Snapshots != nil {
		cur.Snapshots = d.Snapshots
	}
	if d.Enabled != nil {
		cur.Enabled = d.Enabled
	}
	if d.Motion != nil {
		cur.Motion = d.Motion
	}
	if d.RecordConfigured != nil {
		cur.RecordConfigured = d.RecordConfigured
	}
	if d.HasHQ != nil {
		cur.HasHQ = d.HasHQ
	}
	if d.Managed != nil {
		cur.Managed = d.Managed
	}
	if d.Promoted != nil {
		cur.Promoted = d.Promoted
	}
}

