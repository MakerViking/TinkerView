import { useEffect, useState, type ReactNode } from 'react'
import { Modal } from '@/components/Modal'
import { useWs } from '@/ws/WebSocketProvider'
import { Field, Actions, inputCls } from '@/components/devices/forms'
import { Skeleton, Switch } from '@/components/ui'
import { ZoneEditor } from './ZoneEditor'
import type { CameraSettings, Zone } from '@/types/ws'
import { cn } from '@/lib/utils'

const TRACK_OPTS = ['person', 'car', 'bicycle', 'motorcycle', 'truck', 'cat', 'dog', 'bird']
const RECORD_MODES = [
  { value: 'active_objects', label: 'Only when an object is detected (recommended)' },
  { value: 'motion', label: 'On any motion' },
  { value: 'all', label: 'Continuous (24/7)' },
]
const ROTATIONS = [0, 90, 180, 270]

// Chip toggle for tracked-object selection.
export function chipCls(on: boolean): string {
  return cn(
    'rounded-md px-2 py-1 text-xs font-medium transition-colors',
    on
      ? 'bg-ink-800 text-ink-50'
      : 'bg-ink-800 text-ink-400 hover:bg-ink-700 hover:text-ink-200',
  )
}

// CameraSettingsModal edits a camera's Frigate-NVR config: detection, record mode
// (object-only recording), retain days, snapshots, tracked objects, and detection
// zones. Current settings are fetched first so a save doesn't wipe existing zones.
// In imageOnly mode (hand-authored / non-managed cameras) only the rotate/flip
// "Image" controls are shown — the dashboard doesn't own those cameras' detect/
// record config and must not overwrite it.
export function CameraSettingsModal({
  name,
  imageOnly = false,
  onClose,
}: {
  name: string
  imageOnly?: boolean
  onClose: () => void
}) {
  const { send } = useWs()
  const [loaded, setLoaded] = useState(false)
  const [detect, setDetect] = useState(true)
  const [record, setRecord] = useState(true)
  const [recordMode, setRecordMode] = useState('active_objects')
  const [recordDays, setRecordDays] = useState(7)
  const [snapshots, setSnapshots] = useState(true)
  const [objects, setObjects] = useState<string[]>(['person', 'car'])
  const [zones, setZones] = useState<Zone[]>([])
  const [rotate, setRotate] = useState(0)
  const [flipH, setFlipH] = useState(false)
  const [flipV, setFlipV] = useState(false)

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
          setRotate(s.rotate ?? 0)
          setFlipH(s.flipH ?? false)
          setFlipV(s.flipV ?? false)
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
    const settings: CameraSettings = {
      detect,
      record,
      recordMode,
      recordDays,
      snapshots,
      objects,
      zones,
      rotate,
      flipH,
      flipV,
    }
    send({ type: 'camera.settings', data: { name, settings } })
    onClose()
  }

  const imageSection = (
    <>
      <Field label="Rotate" hint="Correct a sideways or upside-down camera">
        <div className="mt-1.5 flex flex-wrap gap-1.5">
          {ROTATIONS.map((deg) => (
            <button type="button" key={deg} onClick={() => setRotate(deg)} className={chipCls(rotate === deg)}>
              {deg}°
            </button>
          ))}
        </div>
      </Field>
      <Row label="Flip horizontal" hint="Mirror left ↔ right">
        <Switch on={flipH} onChange={setFlipH} label="Flip horizontal" />
      </Row>
      <Row label="Flip vertical" hint="Mirror top ↔ bottom">
        <Switch on={flipV} onChange={setFlipV} label="Flip vertical" />
      </Row>
      <p className="-mt-1 text-[12px] text-ink-500">
        Rotate/flip re-encodes the stream (an ffmpeg transcode), so it applies to the live view and
        recordings alike.
      </p>
    </>
  )

  return (
    <Modal size="xl" title={`${imageOnly ? 'Image' : 'Recording & detection'} — ${name}`} onClose={onClose}>
      {!loaded ? (
        <div className="flex flex-col gap-3" aria-label="Loading settings">
          <Skeleton className="h-9" />
          <Skeleton className="h-9" />
          <Skeleton className="h-40" />
        </div>
      ) : (
        <form
          onSubmit={(e) => {
            e.preventDefault()
            submit()
          }}
          className="flex flex-col gap-4"
        >
          {imageOnly ? (
            imageSection
          ) : (
            <>
              <Row label="Object detection" hint="Run the AI detector on this camera">
                <Switch on={detect} onChange={setDetect} label="Object detection" />
              </Row>
              {detect && (
                <Field label="Track these objects">
                  <div className="mt-1.5 flex flex-wrap gap-1.5">
                    {TRACK_OPTS.map((o) => (
                      <button type="button" key={o} onClick={() => toggleObj(o)} className={chipCls(objects.includes(o))}>
                        {o}
                      </button>
                    ))}
                  </div>
                </Field>
              )}

              <Row label="Record" hint="Save video to the NVR">
                <Switch on={record} onChange={setRecord} label="Record" />
              </Row>
              {record && (
                <div className="grid gap-3 sm:grid-cols-2">
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
                </div>
              )}

              <Row label="Snapshots" hint="Save a still image per detection">
                <Switch on={snapshots} onChange={setSnapshots} label="Snapshots" />
              </Row>

              {detect && (
                <Field
                  label="Detection zones (areas of interest)"
                  hint="Limit detection to part of the frame — e.g. the yard, not the street"
                >
                  <div className="mt-1.5">
                    <ZoneEditor cameraName={name} zones={zones} onChange={setZones} />
                  </div>
                </Field>
              )}

              <div className="border-t border-ink-800 pt-3">
                <div className="mb-1.5 text-[13px] font-medium text-ink-200">
                  Image
                </div>
                {imageSection}
              </div>
            </>
          )}

          <Actions onCancel={onClose} submitLabel="Save & apply" />
          <p className="-mt-2 text-right text-[12px] text-ink-500">
            {imageOnly
              ? 'Applying restarts Frigate (~25s) for recorded cameras.'
              : record
                ? 'Applying to a recording camera restarts Frigate (~25s).'
                : 'A rotate/flip change re-applies the stream.'}
          </p>
        </form>
      )}
    </Modal>
  )
}

function Row({ label, hint, children }: { label: string; hint?: string; children: ReactNode }) {
  return (
    <div className="flex items-center gap-3 rounded-lg border border-ink-850 bg-ink-900 px-3.5 py-2.5">
      <div className="min-w-0">
        <div className="text-sm font-medium text-ink-100">{label}</div>
        {hint && <div className="text-[12px] text-ink-500">{hint}</div>}
      </div>
      <div className="ml-auto">{children}</div>
    </div>
  )
}
