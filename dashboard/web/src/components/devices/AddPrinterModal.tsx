import { useState } from 'react'
import { Modal } from '@/components/Modal'
import { useWs } from '@/ws/WebSocketProvider'
import { useLive } from '@/store/live'
import type { PrinterInput, PrinterKind, PrinterState } from '@/types/ws'
import { Actions, Field, inputCls } from './forms'

const KINDS: { value: PrinterKind; label: string; disabled?: boolean }[] = [
  { value: 'moonraker', label: 'Klipper / Moonraker' },
  { value: 'octoprint', label: 'OctoPrint (adapter not built yet)', disabled: true },
  { value: 'bambu', label: 'Bambu Lab (adapter not built yet)', disabled: true },
]

// AddPrinterModal adds a new printer or edits an existing one. On edit, connection
// fields are left blank and "kept" server-side unless changed (so secrets are not
// shipped to the browser).
export function AddPrinterModal({
  existing,
  onClose,
}: {
  existing?: PrinterState
  onClose: () => void
}) {
  const { send } = useWs()
  // Select the stable cameras object, then derive the sorted names in render — a
  // selector returning a fresh array each call makes Zustand's store loop (React #185).
  const cameras = Object.keys(useLive((s) => s.cameras)).sort()
  const editing = !!existing

  const [form, setForm] = useState<PrinterInput>({
    id: existing?.id ?? '',
    kind: existing?.kind ?? 'moonraker',
    name: existing?.name ?? '',
    baseUrl: '',
    apiKey: '',
    ip: '',
    serial: '',
    accessCode: '',
    model: '',
    camera: existing?.camera ?? '',
    uiUrl: existing?.uiUrl ?? '',
  })
  const set = (k: keyof PrinterInput, v: string) => setForm((f) => ({ ...f, [k]: v }))
  // Curated macro allowlist as comma-separated text (e.g. "T0,T1,T2,T3,M600").
  const [macrosText, setMacrosText] = useState((existing?.caps?.macros ?? []).join(', '))

  const submit = () => {
    const printer: PrinterInput = { ...form }
    if (form.kind === 'moonraker') {
      printer.macros = macrosText
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean)
    }
    if (editing) {
      send({ type: 'printer.edit', data: { id: existing!.id, printer } })
    } else {
      send({ type: 'printer.add', data: { printer } })
    }
    onClose()
  }

  return (
    <Modal title={editing ? `Edit ${existing!.name}` : 'Add printer'} onClose={onClose}>
      <form
        onSubmit={(e) => {
          e.preventDefault()
          submit()
        }}
        className="flex flex-col gap-3"
      >
        <Field label="Type">
          <select
            className={inputCls}
            value={form.kind}
            onChange={(e) => set('kind', e.target.value)}
            disabled={editing}
          >
            {KINDS.map((k) => (
              <option key={k.value} value={k.value} disabled={k.disabled}>
                {k.label}
              </option>
            ))}
          </select>
        </Field>

        <div className="grid grid-cols-2 gap-3">
          <Field label="ID" hint={editing ? 'cannot be changed' : 'lowercase, e.g. voron'}>
            <input
              className={inputCls}
              value={form.id}
              onChange={(e) => set('id', e.target.value)}
              disabled={editing}
              required
              placeholder="voron"
            />
          </Field>
          <Field label="Name">
            <input
              className={inputCls}
              value={form.name}
              onChange={(e) => set('name', e.target.value)}
              required
              placeholder="Voron 2.4"
            />
          </Field>
        </div>

        {form.kind === 'moonraker' && (
          <>
            <Field
              label="Base URL"
              hint={editing ? 'leave blank to keep current' : 'http://192.168.0.76 or :7125'}
            >
              <input
                className={inputCls}
                value={form.baseUrl}
                onChange={(e) => set('baseUrl', e.target.value)}
                placeholder="http://192.168.0.76"
              />
            </Field>
            <Field label="API key" hint="optional; leave blank to keep / if none">
              <input
                className={inputCls}
                value={form.apiKey}
                onChange={(e) => set('apiKey', e.target.value)}
                placeholder="(optional)"
              />
            </Field>
            <div className="grid grid-cols-2 gap-3">
              <Field label="Camera stream" hint="optional">
                <select className={inputCls} value={form.camera} onChange={(e) => set('camera', e.target.value)}>
                  <option value="">(none)</option>
                  {cameras.map((c) => (
                    <option key={c} value={c}>
                      {c}
                    </option>
                  ))}
                </select>
              </Field>
              <Field label="Web UI URL" hint="optional; defaults to base URL">
                <input
                  className={inputCls}
                  value={form.uiUrl}
                  onChange={(e) => set('uiUrl', e.target.value)}
                  placeholder="(optional)"
                />
              </Field>
            </div>
            <Field
              label="Macro buttons"
              hint="comma-separated gcode macros to show as buttons (e.g. T0,T1,T2,T3,M600). Blank = a safe default set."
            >
              <input
                className={inputCls}
                value={macrosText}
                onChange={(e) => setMacrosText(e.target.value)}
                placeholder="T0,T1,T2,T3,M600"
              />
            </Field>
          </>
        )}

        {form.kind === 'octoprint' && (
          <>
            <Field label="Base URL" hint={editing ? 'leave blank to keep current' : 'http://192.168.0.x'}>
              <input className={inputCls} value={form.baseUrl} onChange={(e) => set('baseUrl', e.target.value)} />
            </Field>
            <Field label="API key">
              <input className={inputCls} value={form.apiKey} onChange={(e) => set('apiKey', e.target.value)} />
            </Field>
          </>
        )}

        {form.kind === 'bambu' && (
          <div className="grid grid-cols-2 gap-3">
            <Field label="IP">
              <input className={inputCls} value={form.ip} onChange={(e) => set('ip', e.target.value)} />
            </Field>
            <Field label="Serial">
              <input className={inputCls} value={form.serial} onChange={(e) => set('serial', e.target.value)} />
            </Field>
            <Field label="Access code">
              <input className={inputCls} value={form.accessCode} onChange={(e) => set('accessCode', e.target.value)} />
            </Field>
            <Field label="Model">
              <input className={inputCls} value={form.model} onChange={(e) => set('model', e.target.value)} placeholder="x1c" />
            </Field>
          </div>
        )}

        <Actions onCancel={onClose} submitLabel={editing ? 'Save' : 'Add printer'} />
      </form>
    </Modal>
  )
}
