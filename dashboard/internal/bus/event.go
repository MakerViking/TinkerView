package bus

import "time"

// Source identifies the subsystem that emitted an Event.
type Source string

const (
	SourceFrigate Source = "frigate"
	SourcePrinter Source = "printer"
	SourceDocker  Source = "docker"
	SourceSystem  Source = "system"
)

// Kind is the event discriminator within a Source. It maps 1:1 onto the
// server->client WebSocket message types in internal/hub.
type Kind string

const (
	KindCameraState  Kind = "camera.state"
	KindCameraStatus Kind = "camera.status"
	KindPrinterState Kind = "printer.state"
	KindContainers   Kind = "containers"
	KindDisks        Kind = "disks"
	KindFrigate      Kind = "frigate"
	KindAlert        Kind = "alert"
	KindNotice       Kind = "notice"
)

// Event is the single normalized message carried on the bus. Payload is a
// concrete typed value from internal/adapter (CameraState, PrinterState,
// []Container, Alert, ...); subscribers type-switch on Kind.
type Event struct {
	Source  Source
	Kind    Kind
	ID      string // entity id: camera name, printer id, container id, ...
	Ts      time.Time
	Payload any
}
