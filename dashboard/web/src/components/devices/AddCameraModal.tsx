import { useState } from 'react'
import { Modal } from '@/components/Modal'
import { useWs } from '@/ws/WebSocketProvider'
import type { CameraSrcKind } from '@/types/ws'
import { Actions, Field, inputCls } from './forms'
import { Switch } from '@/components/ui'

const SRC_KINDS: { value: CameraSrcKind; label: string; hint: string }[] = [
  { value: 'rtsp', label: 'RTSP (H.264)', hint: 'rtsp://host:554/stream — passthrough; put the login in the fields below' },
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
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
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
          username: username.trim() || undefined,
          password: password || undefined,
        },
      })
    } else {
      send({
        type: 'camera.add',
        data: { name, srcKind, srcInput, username: username.trim() || undefined, password: password || undefined },
      })
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
        className="flex flex-col gap-4"
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

        {srcKind !== 'advanced' && (
          <div className="grid grid-cols-2 gap-3">
            <Field label="Username" hint={editing ? 'blank = keep current' : 'optional'}>
              <input
                className={inputCls}
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                autoComplete="off"
                placeholder={editing ? '••••••' : 'admin'}
              />
            </Field>
            <Field label="Password" hint={editing ? 'blank = keep current' : 'optional'}>
              <input
                type="password"
                className={inputCls}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="new-password"
                placeholder={editing ? '••••••' : ''}
              />
            </Field>
          </div>
        )}

        {!editing && (
          <div className="rounded-xl border border-ink-800 bg-ink-950/40 p-3.5">
            <div className="flex items-center gap-3">
              <div className="min-w-0">
                <div className="text-sm font-medium text-ink-100">Enable Frigate recording / NVR</div>
                <div className="text-[11px] text-ink-500">restarts Frigate ~25s</div>
              </div>
              <span className="ml-auto">
                <Switch on={promote} onChange={setPromote} label="Enable Frigate recording" />
              </span>
            </div>
            {promote && (
              <div className="mt-3 flex flex-col gap-2.5 border-t border-ink-800 pt-3 animate-fade">
                <label className="flex items-center gap-2.5 text-sm text-ink-200">
                  <Switch size="sm" on={record} onChange={setRecord} />
                  Record
                </label>
                <span className="-mt-1 pl-9 text-[11px] text-ink-500">
                  Retention follows Frigate&apos;s global record config.
                </span>
                <label className="flex items-center gap-2.5 text-sm text-ink-200">
                  <Switch size="sm" on={detect} onChange={setDetect} />
                  Object detection (person/car — security cams)
                </label>
                <label className="flex items-center gap-2.5 text-sm text-ink-200">
                  <Switch size="sm" on={snapshots} onChange={setSnapshots} />
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
