import { useEffect, useState } from 'react'
import { Modal } from '@/components/Modal'
import type { CameraSrcKind } from '@/types/ws'
import type { CameraDetail } from '@/types/devices'
import { apiGet, apiSend } from '@/lib/api'
import { migrateHqPref } from '@/lib/hqPref'
import { Actions, Field, FormError, FormSkeleton, inputCls } from './forms'
import { SRC_KINDS, validCameraName } from './cameraKinds'

// baseKind is the source kind a loaded camera starts from. Config-defined cameras
// carry no kind (their source is a raw go2rtc string), so they default to advanced.
function baseKind(d: CameraDetail): CameraSrcKind {
  return d.srcKind || (!d.managed || d.configStream ? 'advanced' : 'rtsp')
}

// EditCameraModal edits an existing camera: name, source and login. It loads the
// stored values first (the source is shown without its embedded credentials, the
// password is never sent to the browser), submits via REST, and stays open with the
// error inline if the server rejects the change. Cameras defined in the Frigate
// config can only have their source URL edited (when the server says editable);
// promoted (NVR) cameras can't be renamed without demoting first.
export function EditCameraModal({ name: current, onClose }: { name: string; onClose: () => void }) {
  const [detail, setDetail] = useState<CameraDetail | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)

  const [name, setName] = useState(current)
  const [srcKind, setSrcKind] = useState<CameraSrcKind>('rtsp')
  const [srcInput, setSrcInput] = useState('')
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [removeLogin, setRemoveLogin] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    void apiGet<CameraDetail>(`/api/cameras/${encodeURIComponent(current)}`).then((r) => {
      if (cancelled) return
      if (r.error !== undefined) {
        setLoadError(r.error)
        return
      }
      const d = r.data
      setDetail(d)
      setName(d.name)
      setSrcKind(baseKind(d))
      setSrcInput(d.srcInput ?? '')
      setUsername(d.username ?? '')
    })
    return () => {
      cancelled = true
    }
  }, [current])

  const configDefined = !!detail && (!detail.managed || detail.configStream)
  const nameLocked = !!detail && (detail.promoted || configDefined)
  const nameReason = detail?.promoted ? 'Disable NVR to rename' : 'Defined in the Frigate config'
  // Config-defined cameras: source URL only, and only when the server allows it.
  const sourceLocked = !!detail && configDefined && !detail.editable
  const kindLocked = configDefined
  const loginEditable = !!detail && !configDefined && srcKind !== 'advanced'
  const hasLogin = !!detail && (detail.hasPassword || detail.username !== '')

  const nameChanged = !!detail && name !== detail.name
  const sourceChanged = !!detail && (srcInput.trim() !== detail.srcInput || srcKind !== baseKind(detail))
  // An omitted/blank username means "keep" on the server, so emptying the field is
  // not a change; clearing a login goes through Remove login.
  const usernameChanged = username.trim() !== '' && username.trim() !== detail?.username
  const loginChanged = !!detail && loginEditable && (removeLogin || usernameChanged || password !== '')
  // Switching a managed camera that has a login to Advanced hides the login inputs
  // (the raw URL carries its own), so the stored credentials are dropped with it.
  const dropsLogin =
    !!detail && !configDefined && hasLogin && srcKind === 'advanced' && baseKind(detail) !== 'advanced'
  const dirty = nameChanged || sourceChanged || loginChanged
  const valid = !!detail && validCameraName(name) && srcInput.trim() !== ''
  const restartsFrigate = !!detail?.promoted && (sourceChanged || loginChanged)

  const submit = async () => {
    if (!detail || busy || !dirty || !valid) return
    setBusy(true)
    setError(null)
    const body: Record<string, unknown> = {}
    if (nameChanged) body.newName = name
    if (sourceChanged) {
      // Config-defined cameras have no kind; send the URL alone.
      if (!configDefined) body.newSrcKind = srcKind
      body.newSrcInput = srcInput.trim()
    }
    if (dropsLogin) {
      body.clearCredentials = true
    } else if (loginEditable) {
      if (removeLogin) {
        body.clearCredentials = true
      } else {
        if (usernameChanged) body.username = username.trim()
        if (password !== '') body.password = password
      }
    }
    const err = await apiSend('PUT', `/api/cameras/${encodeURIComponent(detail.name)}`, body)
    setBusy(false)
    if (err) {
      setError(err)
      return
    }
    if (nameChanged) migrateHqPref(detail.name, name)
    onClose()
  }

  const hint = SRC_KINDS.find((k) => k.value === srcKind)?.hint

  return (
    <Modal title={`Edit camera ${current}`} onClose={onClose}>
      {loadError ? (
        <div className="flex flex-col gap-4">
          <FormError message={`Couldn't load camera: ${loadError}`} />
          <Actions onCancel={onClose} submitLabel="Save" disabled />
        </div>
      ) : !detail ? (
        <FormSkeleton rows={4} />
      ) : (
        <form
          onSubmit={(e) => {
            e.preventDefault()
            void submit()
          }}
          className="flex flex-col gap-4"
        >
          <Field
            label="Name"
            hint={nameLocked ? nameReason : 'lowercase letters, digits and _ (e.g. garage, front_door)'}
          >
            <input
              className={inputCls}
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={nameLocked}
              required
            />
          </Field>

          <Field label="Source type" hint={kindLocked ? 'Defined in the Frigate config' : undefined}>
            <select
              className={inputCls}
              value={srcKind}
              disabled={kindLocked}
              onChange={(e) => setSrcKind(e.target.value as CameraSrcKind)}
            >
              {SRC_KINDS.map((k) => (
                <option key={k.value} value={k.value}>
                  {k.label}
                </option>
              ))}
            </select>
          </Field>

          <Field
            label="Source URL"
            hint={sourceLocked ? 'Defined in the Frigate config — edit it there' : hint}
          >
            <input
              className={inputCls}
              value={srcInput}
              onChange={(e) => setSrcInput(e.target.value)}
              disabled={sourceLocked}
              required
              placeholder="rtsp://192.168.0.9:554/stream"
            />
          </Field>

          {loginEditable && (
            <div className="flex flex-col gap-2">
              <div className="grid grid-cols-2 gap-3">
                <Field label="Username" hint={hasLogin ? 'use Remove login to clear' : 'optional'}>
                  <input
                    className={inputCls}
                    value={username}
                    onChange={(e) => setUsername(e.target.value)}
                    disabled={removeLogin}
                    autoComplete="off"
                    placeholder="admin"
                  />
                </Field>
                <Field label="Password" hint={detail.hasPassword ? 'leave blank to keep' : 'optional'}>
                  <input
                    type="password"
                    className={inputCls}
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    disabled={removeLogin}
                    autoComplete="new-password"
                    placeholder={detail.hasPassword ? '••••••' : ''}
                  />
                </Field>
              </div>
              {hasLogin && (
                <label className="flex items-center gap-2 text-sm text-ink-200">
                  <input
                    type="checkbox"
                    checked={removeLogin}
                    onChange={(e) => setRemoveLogin(e.target.checked)}
                    className="accent-brand-500"
                  />
                  Remove login
                </label>
              )}
            </div>
          )}

          {dropsLogin && (
            <p className="text-[12px] text-amber-300">
              Advanced sources carry their own login — the saved username and password will be removed.
            </p>
          )}

          {restartsFrigate && (
            <p className="text-[12px] text-amber-300">
              This camera records to Frigate NVR — saving restarts Frigate (~25s).
            </p>
          )}

          <FormError message={error} />
          <Actions onCancel={onClose} busy={busy} disabled={!dirty || !valid} submitLabel="Save" />
        </form>
      )}
    </Modal>
  )
}
