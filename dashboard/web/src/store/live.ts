import { create } from 'zustand'
import type {
  Alert,
  CameraState,
  Container,
  DiskUsage,
  PrinterState,
  ServerMsg,
  Snapshot,
} from '@/types/ws'

interface LiveState {
  connected: boolean
  cameras: Record<string, CameraState>
  printers: Record<string, PrinterState>
  containers: Container[]
  disks: DiskUsage[]
  frigateOnline: boolean
  alerts: Alert[]
  dataWritable: boolean
  lastNotice?: { level: string; msg: string; ts: number }
  setConnected: (b: boolean) => void
  applyServerMsg: (m: ServerMsg) => void
}

export const useLive = create<LiveState>((set) => ({
  connected: false,
  cameras: {},
  printers: {},
  containers: [],
  disks: [],
  frigateOnline: false,
  alerts: [],
  dataWritable: true,
  setConnected: (b) => set({ connected: b }),
  applyServerMsg: (m) => set((s) => reduce(s, m)),
}))

function reduce(s: LiveState, m: ServerMsg): Partial<LiveState> {
  switch (m.type) {
    case 'snapshot': {
      const d = m.data as Snapshot
      return {
        cameras: d.cameras ?? {},
        printers: d.printers ?? {},
        containers: d.containers ?? [],
        disks: d.disks ?? [],
        frigateOnline: d.frigateOnline,
        alerts: d.alerts ?? [],
        dataWritable: d.dataWritable !== false, // undefined (older server) → assume writable
      }
    }
    case 'camera.state': {
      const name = m.camera
      return { cameras: { ...s.cameras, [name]: { ...s.cameras[name], ...m.data, name } } }
    }
    case 'camera.delete': {
      const { [m.camera]: _removed, ...rest } = s.cameras
      void _removed
      return { cameras: rest }
    }
    case 'printer.state':
      return { printers: { ...s.printers, [m.data.id]: m.data } }
    case 'printer.delete': {
      const { [m.data.id]: _removed, ...rest } = s.printers
      void _removed
      return { printers: rest }
    }
    case 'containers':
      return { containers: m.data }
    case 'disks':
      return { disks: m.data }
    case 'frigate':
      return { frigateOnline: m.data.online }
    case 'alert':
      return { alerts: [m.data, ...s.alerts].slice(0, 100) }
    case 'notice':
      return { lastNotice: { level: m.level, msg: m.msg, ts: Date.now() } }
    default:
      return {}
  }
}
