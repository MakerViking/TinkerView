package hub

import (
	"encoding/json"

	"github.com/MakerViking/TinkerView/internal/adapter"
	"github.com/MakerViking/TinkerView/internal/devices"
)

// ServerMsg is the envelope for every server->client message. Only the fields
// relevant to Type are populated (the rest are omitempty). The discriminator is
// `type`. Mirrored in web/src/types/ws.ts (single source of truth).
type ServerMsg struct {
	Type   string `json:"type"`
	V      int    `json:"v,omitempty"`
	Data   any    `json:"data,omitempty"`
	Camera string `json:"camera,omitempty"`
	Role   string `json:"role,omitempty"`
	Value  string `json:"value,omitempty"`
	Level  string `json:"level,omitempty"`
	Msg    string `json:"msg,omitempty"`
	Ts     int64  `json:"ts,omitempty"`
}

// Snapshot is the full authoritative state, sent on connect and on
// snapshot.request. Deltas follow as individual ServerMsgs.
type Snapshot struct {
	Cameras       map[string]adapter.CameraState  `json:"cameras"`
	Printers      map[string]adapter.PrinterState `json:"printers"`
	Containers    []adapter.Container             `json:"containers"`
	Disks         []adapter.DiskUsage             `json:"disks"`
	FrigateOnline bool                            `json:"frigateOnline"`
	Alerts        []adapter.Alert                 `json:"alerts"`
	// DataWritable is false when the writable data dir (devices store) can't be
	// written — UI add/edit of devices will fail until it's fixed (e.g. chown to the
	// container uid). Surfaced as a persistent banner. Defaults true.
	DataWritable bool `json:"dataWritable"`
}

// Notice is the user-facing backend message type. It now lives in
// internal/adapter (adapter.Notice) so managers can publish notices over the bus
// without importing this package; this alias keeps existing references working.
type Notice = adapter.Notice

// ----- client -> server -----

// ClientMsg is the inbound envelope; Data is decoded per Type.
type ClientMsg struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
	Ts   int64           `json:"ts,omitempty"`
}

type CameraToggleCmd struct {
	CmdID   string `json:"cmdId,omitempty"`
	Camera  string `json:"camera"`
	Feature string `json:"feature"`
	Value   bool   `json:"value"`
	Persist bool   `json:"persist,omitempty"`
}

type PrinterCmd struct {
	CmdID string `json:"cmdId,omitempty"`
	ID    string `json:"id"`
	Cmd   string `json:"cmd"`
}

type PrinterTempCmd struct {
	CmdID  string  `json:"cmdId,omitempty"`
	ID     string  `json:"id"`
	Heater string  `json:"heater"`
	Target float64 `json:"target"`
}

type PrinterControlCmd struct {
	CmdID  string                `json:"cmdId,omitempty"`
	ID     string                `json:"id"`
	Action adapter.PrinterAction `json:"action"`
}

type ContainerRestartCmd struct {
	CmdID string `json:"cmdId,omitempty"`
	ID    string `json:"id"`
}

// ----- device CRUD (client -> server) -----

type PrinterAddCmd struct {
	CmdID   string                `json:"cmdId,omitempty"`
	Printer devices.PrinterRecord `json:"printer"`
}

type PrinterEditCmd struct {
	CmdID   string                `json:"cmdId,omitempty"`
	ID      string                `json:"id"`
	Printer devices.PrinterRecord `json:"printer"`
}

type PrinterRemoveCmd struct {
	CmdID string `json:"cmdId,omitempty"`
	ID    string `json:"id"`
}

type CameraAddCmd struct {
	CmdID    string `json:"cmdId,omitempty"`
	Name     string `json:"name"`
	SrcKind  string `json:"srcKind"`
	SrcInput string `json:"srcInput"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type CameraEditCmd struct {
	CmdID       string `json:"cmdId,omitempty"`
	Name        string `json:"name"`
	NewName     string `json:"newName,omitempty"`
	NewSrcKind  string `json:"newSrcKind,omitempty"`
	NewSrcInput string `json:"newSrcInput,omitempty"`
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
}

type CameraRemoveCmd struct {
	CmdID string `json:"cmdId,omitempty"`
	Name  string `json:"name"`
}

type CameraPromoteCmd struct {
	CmdID      string `json:"cmdId,omitempty"`
	Name       string `json:"name"`
	Detect     bool   `json:"detect"`
	Record     bool   `json:"record"`
	RecordDays int    `json:"recordDays,omitempty"`
	Snapshots  bool   `json:"snapshots"`
}

type CameraDemoteCmd struct {
	CmdID string `json:"cmdId,omitempty"`
	Name  string `json:"name"`
}

type CameraSettingsCmd struct {
	CmdID    string                 `json:"cmdId,omitempty"`
	Name     string                 `json:"name"`
	Settings devices.CameraSettings `json:"settings"`
}
