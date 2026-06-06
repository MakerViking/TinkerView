// Package fake provides demo data sources so the dashboard can be run and the UI
// verified end-to-end BEFORE any real printer/camera is configured. Enabled with
// DASHBOARD_DEMO=1 (or automatically when no printers are configured). It is not
// used in a real deployment.
package fake

import (
	"context"
	"math"
	"time"

	"github.com/MakerViking/TinkerView/internal/adapter"
	"github.com/MakerViking/TinkerView/internal/bus"
)

// Printer is a synthetic adapter.Printer that animates a fake print and responds
// to pause/resume/cancel, proving the WS command->echo round-trip.
type Printer struct {
	bus   *bus.Bus
	state adapter.PrinterState
}

func NewPrinter(b *bus.Bus, id, name string) *Printer {
	return &Printer{
		bus: b,
		state: adapter.PrinterState{
			ID: id, Kind: "moonraker", Name: name, Online: true,
			State: adapter.Printing, Filename: "3DBenchy.gcode", TotalLayers: 200,
			Flags: adapter.Flags{CanPause: true, CanCancel: true},
			// Demo caps so the expanded controls render with DASHBOARD_DEMO=1.
			Caps: &adapter.Caps{
				SetTemps: true, SpeedFlow: true, Babystep: true, Move: true,
				EmergencyStop: true, FirmwareRestart: true,
				Macros: []string{"BED_MESH_CALIBRATE"},
			},
			Motion: &adapter.Motion{SpeedFactor: 100, FlowFactor: 100},
		},
	}
}

func (p *Printer) ID() string                     { return p.state.ID }
func (p *Printer) Kind() string                   { return p.state.Kind }
func (p *Printer) Snapshot() adapter.PrinterState { return p.state }

func (p *Printer) Command(_ context.Context, cmd adapter.PrinterCommand) error {
	switch cmd {
	case adapter.CmdPause:
		p.state.State = adapter.Paused
		p.state.Flags = adapter.Flags{CanResume: true, CanCancel: true}
	case adapter.CmdResume:
		p.state.State = adapter.Printing
		p.state.Flags = adapter.Flags{CanPause: true, CanCancel: true}
	case adapter.CmdCancel:
		p.state.State = adapter.Cancelled
		p.state.Flags = adapter.Flags{}
	}
	p.publish()
	return nil
}

func (p *Printer) SetTemp(_ context.Context, heater string, target float64) error {
	if heater == "bed" {
		p.state.Bed.Target = target
	} else {
		p.state.Nozzle.Target = target
	}
	p.publish()
	return nil
}

// Control echoes speed/flow into the demo state so the sliders feel live; other
// actions are accepted as no-ops.
func (p *Printer) Control(_ context.Context, a adapter.PrinterAction) error {
	if p.state.Motion == nil {
		p.state.Motion = &adapter.Motion{SpeedFactor: 100, FlowFactor: 100}
	}
	switch a.Op {
	case adapter.ActSpeed:
		p.state.Motion.SpeedFactor = a.Pct
	case adapter.ActFlow:
		p.state.Motion.FlowFactor = a.Pct
	}
	p.publish()
	return nil
}

func (p *Printer) Run(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	start := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			if p.state.State == adapter.Printing {
				el := now.Sub(start).Seconds()
				p.state.ProgressPct = math.Min(100, el/600*100)
				p.state.ElapsedSec = int(el)
				p.state.EtaSec = int(math.Max(0, 600-el))
				p.state.Layer = int(p.state.ProgressPct / 100 * float64(p.state.TotalLayers))
				p.state.Nozzle = adapter.Heater{Actual: 210 + 2*math.Sin(el/5), Target: 210}
				p.state.Bed = adapter.Heater{Actual: 60, Target: 60}
				if p.state.ProgressPct >= 100 {
					p.state.State = adapter.Complete
					p.state.Flags = adapter.Flags{}
				}
			}
			p.publish()
		}
	}
}

func (p *Printer) publish() {
	p.bus.Publish(bus.Event{
		Source: bus.SourcePrinter, Kind: bus.KindPrinterState,
		ID: p.state.ID, Ts: time.Now(), Payload: p.state,
	})
}

// Cameras periodically publishes fake CameraState + a "Frigate online" signal so
// the camera wall renders tiles for the given go2rtc stream names. The actual
// video still comes live from the real go2rtc via the proxy.
type Cameras struct {
	bus   *bus.Bus
	names []string
}

func NewCameras(b *bus.Bus, names []string) *Cameras {
	return &Cameras{bus: b, names: names}
}

func (c *Cameras) Run(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	c.publish()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			c.publish()
		}
	}
}

func (c *Cameras) publish() {
	yes := true
	c.bus.Publish(bus.Event{Source: bus.SourceFrigate, Kind: bus.KindFrigate, ID: "frigate", Ts: time.Now(), Payload: true})
	for _, n := range c.names {
		c.bus.Publish(bus.Event{
			Source: bus.SourceFrigate, Kind: bus.KindCameraState, ID: n, Ts: time.Now(),
			Payload: adapter.CameraState{Name: n, Detect: &yes, RecordConfigured: &yes},
		})
	}
}
