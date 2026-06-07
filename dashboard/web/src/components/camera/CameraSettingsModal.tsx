import { useEffect, useState, type ReactNode } from 'react'
import { Modal } from '@/components/Modal'
import { useWs } from '@/ws/WebSocketProvider'
import { Field, Actions, inputCls } from '@/components/devices/forms'
import { ZoneEditor } from './ZoneEditor'
import type { CameraSettings, Zone } from '@/types/ws'
import { cn } from '@/lib/utils'

const TRACK_OPTS = ['person', 'car', 'bicycle', 'motorcycle', 'truck', 'cat', 'dog', 'bird']
const RECORD_MODES = [
  { value: 'active_objects', label: 'Only when an object is detected (recommended)' },
  { value: 'motion', label: 'On any motion' },
  { value: 'all', label: 'Continuous (24/7)' },
]

// CameraSettingsModal edits a camera's Frigate-NVR config: detection, record mode
// (object-only recording), retain days, snapshots, tracked objects, and detection
// zones. Current settings are fetched first so a save doesn't wipe existing zones.
export function CameraSettingsModal({ name, onClose }: { name: string; onClose: () => void }) {
  const { send } = useWs()
  const [loaded, setLoaded] = useState(false)
  const [detect, setDetect] = useState(true)
  const [record, setRecord] = useState(true)
  const [recordMode, setRecordMode] = useState('active_objects')
  const [recordDays, setRecordDays] = useState(7)
  const [snapshots, setSnapshots] = useState(true)
  const [objects, setObjects] = useState<string[]>(['person', 'car'])
  const [zones, setZones] = useState<Zone[]>([])

  useEffect(() => {
    let cancelled = false
    fetch(`/api/cameras/${encodeURIComponent(name)}/settings`)
      .then((r) => (r.ok ? r.json() : null))
      .then((s: CameraSettings | null) => {
        if (cancelled) return
        if (s) {
          setDetect(s.detect)
          setRecord(s.record)
          setRecordMode(s.recordMode || 'active_objects')
          setRecordDays(s.recordDays || 7)
          setSnapshots(s.snapshots)
          setObjects(s.objects?.length ? s.objects : ['person', 'car'])
          setZones(s.zones ?? [])
        }
        setLoaded(true)
      })
      .catch(() => setLoaded(true))
    return () => {
      cancelled = true
    }
  }, [name])

  const toggleObj = (o: string) =>
    setObjects((cur) => (cur.includes(o) ? cur.filter((x) => x !== o) : [...cur, o]))

  const submit = () => {
    const settings: CameraSettings = { detect, record, recordMode, recordDays, snapshots, objects, zones }
    send({ type: 'camera.settings', data: { name, settings } })
    onClose()
  }

  return (
    <Modal size="xl" title={`Recording & detection — ${name}`} onClose={onClose}>
      {!loaded ? (
        <div className="p-6 text-center text-sm text-slate-400">loading…</div>
      ) : (
        <form
          onSubmit={(e) => {
            e.preventDefault()
            submit()
          }}
          className="flex flex-col gap-3"
        >
          <Row label="Object detection" hint="Run the AI detector on this camera">
            <Toggle on={detect} onChange={setDetect} />
          </Row>
          {detect && (
            <Field label="Track these objects">
              <div className="mt-1 flex flex-wrap gap-1.5">
                {TRACK_OPTS.map((o) => (
                  <button
                    type="button"
                    key={o}
                    onClick={() => toggleObj(o)}
                    className={cn(
                      'rounded px-2 py-0.5 text-xs',
                      objects.includes(o) ? 'bg-emerald-500/30 text-emerald-200' : 'bg-slate-700 text-slate-400',
                    )}
                  >
                    {o}
                  </button>
                ))}
              </div>
            </Field>
          )}

          <Row label="Record" hint="Save video to the NVR">
            <Toggle on={record} onChange={setRecord} />
          </Row>
          {record && (
            <>
              <Field label="When to record">
                <select className={inputCls} value={recordMode} onChange={(e) => setRecordMode(e.target.value)}>
                  {RECORD_MODES.map((m) => (
                    <option key={m.value} value={m.value}>
                      {m.label}
                    </option>
                  ))}
                </select>
              </Field>
              <Field label="Keep recordings for (days)">
                <input
                  type="number"
                  min={1}
                  max={90}
                  className={inputCls}
                  value={recordDays}
                  onChange={(e) => setRecordDays(Math.max(1, Number(e.target.value) || 1))}
                />
              </Field>
            </>
          )}

          <Row label="Snapshots" hint="Save a still image per detection">
            <Toggle on={snapshots} onChange={setSnapshots} />
          </Row>

          {detect && (
            <Field
              label="Detection zones (areas of interest)"
              hint="Limit detection to part of the frame — e.g. the yard, not the street"
            >
              <div className="mt-1">
                <ZoneEditor cameraName={name} zones={zones} onChange={setZones} />
              </div>
            </Field>
          )}

          <Actions onCancel={onClose} submitLabel="Save & apply" />
          {record && <p className="text-[11px] text-slate-500">Applying to a recording camera restarts Frigate (~25s).</p>}
        </form>
      )}
    </Modal>
  )
}

function Row({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  return (
    <div className="flex items-center gap-3">
      <div className="min-w-0">
        <div className="text-sm text-slate-200">{label}</div>
        {hint && <div className="text-[11px] text-slate-500">{hint}</div>}
      </div>
      <div className="ml-auto">{children}</div>
    </div>
  )
}

function Toggle({ on, onChange }: { on: boolean; onChange: (v: boolean) => void }) {
  return (
    <button
      type="button"
      onClick={() => onChange(!on)}
      className={cn('rounded px-3 py-1 text-xs', on ? 'bg-emerald-500/30 text-emerald-200' : 'bg-slate-700/60 text-slate-300')}
    >
      {on ? 'On ●' : 'Off'}
    </button>
  )
}
