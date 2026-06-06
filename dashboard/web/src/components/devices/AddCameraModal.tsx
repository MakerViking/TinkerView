import { useState } from 'react'
import { Modal } from '@/components/Modal'
import { useWs } from '@/ws/WebSocketProvider'
import type { CameraSrcKind } from '@/types/ws'
import { Actions, Field, inputCls } from './forms'

const SRC_KINDS: { value: CameraSrcKind; label: string; hint: string }[] = [
  { value: 'rtsp', label: 'RTSP (H.264)', hint: 'rtsp://user:pass@host:554/stream — passthrough, near-zero CPU' },
  { value: 'mjpeg', label: 'MJPEG printer cam', hint: 'http://host/webcam/?action=stream — transcoded to H.264 for live view' },
  { value: 'h264', label: 'Raw H.264 over HTTP', hint: 'http://host/webcam/stream.h264 (e.g. Snapmaker)' },
  { value: 'mjpeg_raw', label: 'MJPEG (no transcode)', hint: 'Low CPU; falls back to MJPEG/MSE, cannot WebRTC' },
  { value: 'advanced', label: 'Advanced (raw go2rtc source)', hint: 'Power user: full go2rtc source string' },
]

// AddCameraModal adds a camera (instant go2rtc live tile, optionally promoted to
// Frigate NVR) or edits an existing one's name/source.
export function AddCameraModal({
  existingName,
  onClose,
}: {
  existingName?: string
  onClose: () => void
}) {
  const { send } = useWs()
  const editing = !!existingName

  const [name, setName] = useState(existingName ?? '')
  const [srcKind, setSrcKind] = useState<CameraSrcKind>('rtsp')
  const [srcInput, setSrcInput] = useState('')
  // promote-on-add
  const [promote, setPromote] = useState(false)
  const [detect, setDetect] = useState(false)
  const [record, setRecord] = useState(true)
  const [snapshots, setSnapshots] = useState(false)

  const hint = SRC_KINDS.find((k) => k.value === srcKind)?.hint

  const submit = () => {
    if (editing) {
      send({
        type: 'camera.edit',
        data: {
          name: existingName!,
          newName: name !== existingName ? name : undefined,
          newSrcKind: srcInput ? srcKind : undefined,
          newSrcInput: srcInput || undefined,
        },
      })
    } else {
      send({ type: 'camera.add', data: { name, srcKind, srcInput } })
      if (promote) {
        send({ type: 'camera.promote', data: { name, detect, record, snapshots } })
      }
    }
    onClose()
  }

  return (
    <Modal title={editing ? `Edit camera ${existingName}` : 'Add camera'} onClose={onClose}>
      <form
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
        className="flex flex-col gap-3"
      >
        <Field label="Name" hint="lowercase letters, digits and _ (e.g. garage, front_door)">
          <input
            className={inputCls}
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            placeholder="garage"
          />
        </Field>

        <Field label="Source type">
          <select className={inputCls} value={srcKind} onChange={(e) => setSrcKind(e.target.value as CameraSrcKind)}>
            {SRC_KINDS.map((k) => (
              <option key={k.value} value={k.value}>
                {k.label}
              </option>
            ))}
          </select>
        </Field>

        <Field
          label={editing ? 'New source URL' : 'Source URL'}
          hint={editing ? 'leave blank to keep the current source' : hint}
        >
          <input
            className={inputCls}
            value={srcInput}
            onChange={(e) => setSrcInput(e.target.value)}
            required={!editing}
            placeholder="rtsp://192.168.0.9:554/stream"
          />
        </Field>

        {!editing && (
          <div className="rounded-md border border-slate-800 p-3">
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={promote} onChange={(e) => setPromote(e.target.checked)} />
              Enable Frigate recording / NVR
              <span className="text-[11px] text-slate-500">(restarts Frigate ~25s)</span>
            </label>
            {promote && (
              <div className="mt-3 flex flex-col gap-2 pl-1">
                <label className="flex items-center gap-2 text-sm text-slate-300">
                  <input type="checkbox" checked={record} onChange={(e) => setRecord(e.target.checked)} />
                  Record
                </label>
                <span className="text-[11px] text-slate-500">
                  Retention follows Frigate&apos;s global record config.
                </span>
                <label className="flex items-center gap-2 text-sm text-slate-300">
                  <input type="checkbox" checked={detect} onChange={(e) => setDetect(e.target.checked)} />
                  Object detection (person/car — security cams)
                </label>
                <label className="flex items-center gap-2 text-sm text-slate-300">
                  <input type="checkbox" checked={snapshots} onChange={(e) => setSnapshots(e.target.checked)} />
                  Snapshots
                </label>
              </div>
            )}
          </div>
        )}

        <Actions onCancel={onClose} submitLabel={editing ? 'Save' : 'Add camera'} />
      </form>
    </Modal>
  )
}
