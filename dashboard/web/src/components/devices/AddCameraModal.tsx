import { useState } from 'react'
import { Modal } from '@/components/Modal'
import type { CameraSrcKind } from '@/types/ws'
import { apiSend } from '@/lib/api'
import { Actions, Field, FormError, inputCls } from './forms'
import { SRC_KINDS, validCameraName } from './cameraKinds'
import { Switch } from '@/components/ui'

// AddCameraModal adds a camera (instant go2rtc live tile, optionally promoted to
// Frigate NVR). Editing an existing camera lives in EditCameraModal. The add is a
// REST POST so a rejection is shown inline and NVR promotion only follows a
// successful add.
export function AddCameraModal({ onClose }: { onClose: () => void }) {
  const [name, setName] = useState('')
  const [srcKind, setSrcKind] = useState<CameraSrcKind>('rtsp')
  const [srcInput, setSrcInput] = useState('')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  // promote-on-add
  const [promote, setPromote] = useState(false)
  const [detect, setDetect] = useState(false)
  const [record, setRecord] = useState(true)
  const [snapshots, setSnapshots] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  // The camera exists once the add succeeded; a failed promote then retries only
  // the promote instead of re-adding (which would be rejected as a duplicate).
  const [added, setAdded] = useState(false)

  const hint = SRC_KINDS.find((k) => k.value === srcKind)?.hint

  const valid = validCameraName(name) && srcInput.trim() !== ''

  const submit = async () => {
    if (busy || !valid) return
    setBusy(true)
    setError(null)
    if (!added) {
      const err = await apiSend('POST', '/api/cameras', {
        name,
        srcKind,
        srcInput: srcInput.trim(),
        username: username.trim() || undefined,
        password: password || undefined,
      })
      if (err) {
        setError(err)
        setBusy(false)
        return
      }
      setAdded(true)
    }
    if (promote) {
      const err = await apiSend('POST', `/api/cameras/${encodeURIComponent(name)}/promote`, {
        detect,
        record,
        snapshots,
      })
      if (err) {
        setError(`Camera added, but enabling NVR failed: ${err}`)
        setBusy(false)
        return
      }
    }
    onClose()
  }

  return (
    <Modal title="Add camera" onClose={onClose}>
      <form
        onSubmit={(e) => {
          e.preventDefault()
          void submit()
        }}
        className="flex flex-col gap-4"
      >
        <Field label="Name" hint="lowercase letters, digits and _ (e.g. garage, front_door)">
          <input
            className={inputCls}
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            disabled={added}
            placeholder="garage"
          />
        </Field>

        <Field label="Source type">
          <select className={inputCls} disabled={added} value={srcKind} onChange={(e) => setSrcKind(e.target.value as CameraSrcKind)}>
            {SRC_KINDS.map((k) => (
              <option key={k.value} value={k.value}>
                {k.label}
              </option>
            ))}
          </select>
        </Field>

        <Field label="Source URL" hint={hint}>
          <input
            className={inputCls}
            value={srcInput}
            onChange={(e) => setSrcInput(e.target.value)}
            required
            disabled={added}
            placeholder="rtsp://192.168.0.9:554/stream"
          />
        </Field>

        {srcKind !== 'advanced' && (
          <div className="grid grid-cols-2 gap-3">
            <Field label="Username" hint="optional">
              <input
                className={inputCls}
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                autoComplete="off"
                disabled={added}
                placeholder="admin"
              />
            </Field>
            <Field label="Password" hint="optional">
              <input
                type="password"
                className={inputCls}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                autoComplete="new-password"
                disabled={added}
              />
            </Field>
          </div>
        )}

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

        <FormError message={error} />
        <Actions
          onCancel={onClose}
          busy={busy}
          disabled={!valid}
          submitLabel={added ? (promote ? 'Retry enabling NVR' : 'Done') : 'Add camera'}
        />
      </form>
    </Modal>
  )
}
