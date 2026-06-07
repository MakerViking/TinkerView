// Mirrors the Go structs in internal/adapter + internal/hub. Single source of
// truth for the WS protocol; keep in sync when the Go side changes.

export type PState =
  | 'offline'
  | 'idle'
  | 'printing'
  | 'paused'
  | 'complete'
  | 'error'
  | 'cancelled'

export interface Heater {
  actual: number
  target: number
}

export interface Flags {
  canPause: boolean
  canResume: boolean
  canCancel: boolean
  controlBlocked: boolean
}

export interface FanCap {
  key: string
  object: string
  kind: 'part' | 'chamber' | 'generic'
}
export interface Caps {
  setTemps: boolean
  chamber: boolean
  fans?: FanCap[]
  filamentSensor: boolean
  speedFlow: boolean
  babystep: boolean
  move: boolean
  emergencyStop: boolean
  firmwareRestart: boolean
  fileBrowser: boolean
  history: boolean
  macros?: string[]
}
export interface Fan {
  key: string
  speed: number // 0..1
  rpm?: number
}
export interface FilamentSensor {
  name: string
  detected: boolean
  enabled: boolean
}
export interface Motion {
  x: number
  y: number
  z: number
  homedAxes: string
  speedFactor: number // percent
  flowFactor: number // percent
  zOffset: number
}
export interface HostLoad {
  cpuPct: number
  memPct: number
  mcuLoad: number
}

export interface PrinterState {
  id: string
  kind: 'moonraker' | 'octoprint' | 'prusalink' | 'bambu'
  name: string
  online: boolean
  state: PState
  filename: string
  progressPct: number
  etaSec: number
  elapsedSec: number
  nozzle: Heater
  bed: Heater
  chamber?: Heater
  layer: number
  totalLayers: number
  flags: Flags
  message?: string
  camera?: string
  uiUrl?: string
  managed: boolean // dashboard owns this printer's config (UI may edit/remove)
  caps?: Caps
  fans?: Fan[]
  filamentSensor?: FilamentSensor
  motion?: Motion
  hostLoad?: HostLoad
}

// PrinterAction mirrors the Go adapter.PrinterAction tagged union (printer.control).
export interface PrinterAction {
  op:
    | 'fan'
    | 'speed'
    | 'flow'
    | 'babystep'
    | 'estop'
    | 'firmware_restart'
    | 'restart'
    | 'home'
    | 'move'
    | 'extrude'
    | 'macro'
    | 'print_file'
    | 'reprint'
  name?: string // fan key | macro name | file path
  pct?: number // fan/speed/flow percent
  axis?: string // x|y|z for move
  dist?: number // mm (signed) for move/extrude/babystep
  feed?: number // mm/min
  axes?: string // home axes ("" == all)
}

export interface CameraState {
  name: string
  detect?: boolean
  recordings?: boolean
  snapshots?: boolean
  enabled?: boolean
  motion?: boolean
  recordConfigured?: boolean
  hasHq?: boolean
  managed?: boolean // dashboard owns this camera (UI may edit/remove/promote)
  promoted?: boolean // written into Frigate config (NVR), not just a live tile
}

// PrinterInput mirrors the Go devices.PrinterRecord JSON shape (add/edit forms).
export type PrinterKind = 'moonraker' | 'octoprint' | 'prusalink' | 'bambu'
export interface PrinterInput {
  id: string
  kind: PrinterKind
  name: string
  baseUrl?: string
  apiKey?: string
  ip?: string
  serial?: string
  accessCode?: string
  model?: string
  camera?: string
  uiUrl?: string
  macros?: string[]
}

// CameraSrcKind selects how a camera source URL is turned into a go2rtc source.
export type CameraSrcKind = 'rtsp' | 'h264' | 'mjpeg' | 'mjpeg_raw' | 'advanced'

// Zone is a Frigate detection area of interest (a normalized [x,y] polygon).
export interface Zone {
  name: string
  coordinates: number[][]
  objects?: string[]
}

// CameraSettings mirrors the Go devices.CameraSettings JSON shape.
export interface CameraSettings {
  detect: boolean
  record: boolean
  recordMode: string // active_objects | motion | all
  recordDays: number
  snapshots: boolean
  objects?: string[]
  zones?: Zone[]
}

export interface Alert {
  id: string
  kind:
    | 'person'
    | 'motion'
    | 'camera_offline'
    | 'print_done'
    | 'print_failed'
    | 'frigate_down'
  camera?: string
  printerId?: string
  ts: number
  title: string
  message: string
  thumbUrl?: string
  clipUrl?: string
}

export interface Container {
  id: string
  name: string
  state: string
  status: string
  health?: string
  cpuPct: number
  memUsed: number
  memLimit: number
  uptimeSec: number
}

export interface DiskUsage {
  path: string
  total: number
  free: number
  used: number
  usedPct: number
}

export interface Snapshot {
  cameras: Record<string, CameraState>
  printers: Record<string, PrinterState>
  containers: Container[]
  disks: DiskUsage[]
  frigateOnline: boolean
  alerts: Alert[]
}

// ----- server -> client -----
export type ServerMsg =
  | { type: 'snapshot'; v: number; data: Snapshot }
  | { type: 'camera.state'; camera: string; data: Partial<CameraState> }
  | { type: 'camera.delete'; camera: string }
  | { type: 'camera.status'; camera: string; role: string; value: string }
  | { type: 'printer.state'; data: PrinterState }
  | { type: 'printer.delete'; data: { id: string } }
  | { type: 'containers'; data: Container[] }
  | { type: 'disks'; data: DiskUsage[] }
  | { type: 'frigate'; data: { online: boolean } }
  | { type: 'alert'; data: Alert }
  | { type: 'notice'; level: 'info' | 'warn' | 'error'; msg: string }
  | { type: 'pong'; ts: number }

// ----- client -> server -----
export type ClientMsg =
  | { type: 'snapshot.request' }
  | {
      type: 'camera.toggle'
      data: {
        cmdId?: string
        camera: string
        feature: 'detect' | 'recordings' | 'snapshots' | 'enabled' | 'motion'
        value: boolean
        persist?: boolean
      }
    }
  | { type: 'printer.cmd'; data: { cmdId?: string; id: string; cmd: 'pause' | 'resume' | 'cancel' } }
  | { type: 'printer.temp'; data: { cmdId?: string; id: string; heater: string; target: number } }
  | { type: 'printer.control'; data: { cmdId?: string; id: string; action: PrinterAction } }
  | { type: 'container.restart'; data: { cmdId?: string; id: string } }
  | { type: 'printer.add'; data: { cmdId?: string; printer: PrinterInput } }
  | { type: 'printer.edit'; data: { cmdId?: string; id: string; printer: PrinterInput } }
  | { type: 'printer.remove'; data: { cmdId?: string; id: string } }
  | {
      type: 'camera.add'
      data: { cmdId?: string; name: string; srcKind: CameraSrcKind; srcInput: string; username?: string; password?: string }
    }
  | {
      type: 'camera.edit'
      data: {
        cmdId?: string
        name: string
        newName?: string
        newSrcKind?: CameraSrcKind
        newSrcInput?: string
        username?: string
        password?: string
      }
    }
  | { type: 'camera.remove'; data: { cmdId?: string; name: string } }
  | {
      type: 'camera.promote'
      data: { cmdId?: string; name: string; detect: boolean; record: boolean; recordDays?: number; snapshots: boolean }
    }
  | { type: 'camera.demote'; data: { cmdId?: string; name: string } }
  | { type: 'camera.settings'; data: { cmdId?: string; name: string; settings: CameraSettings } }
  | { type: 'ping'; ts: number }
