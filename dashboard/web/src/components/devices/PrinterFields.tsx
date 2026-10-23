import type { PrinterInput, PrinterKind } from '@/types/ws'
import { Field, inputCls } from './forms'

export const PRINTER_KINDS: { value: PrinterKind; label: string; sub: string }[] = [
  { value: 'moonraker', label: 'Klipper', sub: 'Moonraker' },
  { value: 'octoprint', label: 'OctoPrint', sub: 'REST API' },
  { value: 'prusalink', label: 'Prusa', sub: 'PrusaLink' },
  { value: 'bambu', label: 'Bambu Lab', sub: 'LAN mode' },
]

// PrusaLink exposes no gcode/macro endpoint, so macros only apply to the other
// three kinds (their adapters all accept a curated macro list).
export const kindSupportsMacros = (k: PrinterKind) => k !== 'prusalink'

export function parseMacros(text: string): string[] {
  return text
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
}

export function macrosHint(kind: PrinterKind): string {
  return kind === 'moonraker'
    ? 'comma-separated Klipper macros to show as buttons (e.g. T0,T1,T2,T3,M600). Blank = a safe default set.'
    : 'comma-separated gcode/macros to show as buttons (e.g. M600,M104 S0). Blank = a safe default set.'
}

// ConnectionFields renders the per-kind connection inputs shared by the add and
// edit forms. In edit mode the stored secrets are never sent to the browser, so
// their inputs stay blank and mean "keep current" (hasKey/hasCode flag that one is set).
export function ConnectionFields({
  form,
  set,
  editing,
  hasKey,
  hasCode,
}: {
  form: PrinterInput
  set: (k: keyof PrinterInput, v: string) => void
  editing?: boolean
  hasKey?: boolean
  hasCode?: boolean
}) {
  const keep = 'leave blank to keep current'
  return (
    <div className="flex flex-col gap-3">
      {form.kind === 'moonraker' && (
        <>
          <Field label="Base URL" hint={editing ? undefined : 'http://192.168.1.50 or :7125'}>
            <input
              className={inputCls}
              value={form.baseUrl ?? ''}
              onChange={(e) => set('baseUrl', e.target.value)}
              required
              placeholder="http://192.168.1.50"
            />
          </Field>
          <Field label="API key" hint={editing && hasKey ? keep : 'optional; leave blank if none'}>
            <input
              className={inputCls}
              value={form.apiKey ?? ''}
              onChange={(e) => set('apiKey', e.target.value)}
              placeholder={editing && hasKey ? '••••••' : '(optional)'}
              autoComplete="off"
            />
          </Field>
        </>
      )}

      {form.kind === 'octoprint' && (
        <>
          <Field label="Base URL" hint={editing ? undefined : 'http://192.168.1.x'}>
            <input
              className={inputCls}
              value={form.baseUrl ?? ''}
              onChange={(e) => set('baseUrl', e.target.value)}
              required
              placeholder="http://192.168.1.51"
            />
          </Field>
          <Field label="API key" hint={editing ? keep : 'OctoPrint → Settings → Application Keys'}>
            <input
              className={inputCls}
              value={form.apiKey ?? ''}
              onChange={(e) => set('apiKey', e.target.value)}
              required={!editing}
              placeholder={editing && hasKey ? '••••••' : ''}
              autoComplete="off"
            />
          </Field>
        </>
      )}

      {form.kind === 'prusalink' && (
        <>
          <Field label="Base URL" hint={editing ? undefined : "the printer's IP, e.g. http://192.168.1.x"}>
            <input
              className={inputCls}
              value={form.baseUrl ?? ''}
              onChange={(e) => set('baseUrl', e.target.value)}
              required
              placeholder="http://192.168.1.52"
            />
          </Field>
          <Field
            label="Password / API key"
            hint={
              editing
                ? keep
                : 'the PrusaLink password (a.k.a. API key) — printer: Settings → Network → PrusaLink. The login name is always "maker".'
            }
          >
            <input
              className={inputCls}
              value={form.apiKey ?? ''}
              onChange={(e) => set('apiKey', e.target.value)}
              required={!editing}
              placeholder={editing && hasKey ? '••••••' : ''}
              autoComplete="off"
            />
          </Field>
        </>
      )}

      {form.kind === 'bambu' && (
        <div className="grid grid-cols-2 gap-3">
          <Field label="IP">
            <input
              className={inputCls}
              value={form.ip ?? ''}
              onChange={(e) => set('ip', e.target.value)}
              required
              placeholder="192.168.1.53"
            />
          </Field>
          <Field label="Serial">
            <input
              className={inputCls}
              value={form.serial ?? ''}
              onChange={(e) => set('serial', e.target.value)}
              required
            />
          </Field>
          <Field label="Access code" hint={editing ? keep : 'LAN Access Code (printer screen)'}>
            <input
              className={inputCls}
              value={form.accessCode ?? ''}
              onChange={(e) => set('accessCode', e.target.value)}
              required={!editing}
              placeholder={editing && hasCode ? '••••••' : ''}
              autoComplete="off"
            />
          </Field>
          <Field label="Model" hint="x1 | x1c | p1s | p1p | a1 | a1mini">
            <input
              className={inputCls}
              value={form.model ?? ''}
              onChange={(e) => set('model', e.target.value)}
              placeholder="x1c"
            />
          </Field>
        </div>
      )}
    </div>
  )
}

// CameraSelect is the linked-camera dropdown: any camera you've set up, not just a
// printer-mounted one. `current` stays selectable even if it is no longer listed.
export function CameraSelect({
  value,
  onChange,
  cameras,
}: {
  value: string
  onChange: (v: string) => void
  cameras: string[]
}) {
  const opts = value && !cameras.includes(value) ? [...cameras, value] : cameras
  return (
    <Field label="Camera" hint="optional; any camera you've set up">
      <select className={inputCls} value={value} onChange={(e) => onChange(e.target.value)}>
        <option value="">(none)</option>
        {opts.map((c) => (
          <option key={c} value={c}>
            {c}
          </option>
        ))}
      </select>
    </Field>
  )
}
