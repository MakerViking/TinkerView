import { useState } from 'react'
import { Modal } from '@/components/Modal'
import { useWs } from '@/ws/WebSocketProvider'
import { useLive } from '@/store/live'
import type { PrinterInput, PrinterKind, PrinterState } from '@/types/ws'
import { Actions, Field, inputCls } from './forms'
import { cn } from '@/lib/utils'

const KINDS: { value: PrinterKind; label: string; sub: string }[] = [
  { value: 'moonraker', label: 'Klipper', sub: 'Moonraker' },
  { value: 'octoprint', label: 'OctoPrint', sub: 'REST API' },
  { value: 'prusalink', label: 'Prusa', sub: 'PrusaLink' },
  { value: 'bambu', label: 'Bambu Lab', sub: 'LAN mode' },
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

  // PrusaLink exposes no gcode/macro endpoint, so macros only apply to the other
  // three kinds (their adapters all accept a curated macro list).
  const supportsMacros = form.kind !== 'prusalink'

  const submit = () => {
    const printer: PrinterInput = { ...form }
    if (supportsMacros) {
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
        className="flex flex-col gap-4"
      >
        {/* Step 1 — what kind of printer? Card picker instead of a bare select. */}
        <div>
          <span className="font-display text-[11px] font-medium uppercase tracking-[0.08em] text-ink-400">
            Type
          </span>
          <div role="radiogroup" aria-label="Printer type" className="mt-1.5 grid grid-cols-2 gap-2 sm:grid-cols-4">
            {KINDS.map((k) => {
              const active = form.kind === k.value
              return (
                <button
                  type="button"
                  role="radio"
                  aria-checked={active}
                  key={k.value}
                  disabled={editing}
                  onClick={() => set('kind', k.value)}
                  className={cn(
                    'flex flex-col items-start rounded-xl border px-3 py-2.5 text-left transition-colors',
                    active
                      ? 'border-brand-500/60 bg-brand-500/10 shadow-[0_0_12px_-4px_rgb(56_189_248/0.4)]'
                      : 'border-ink-700 bg-ink-950/40 hover:border-ink-500',
                    editing && !active && 'opacity-40',
                    editing && 'cursor-not-allowed',
                  )}
                >
                  <span className={cn('font-display text-sm font-semibold', active ? 'text-brand-300' : 'text-ink-100')}>
                    {k.label}
                  </span>
                  <span className="text-[10px] text-ink-500">{k.sub}</span>
                </button>
              )
            })}
          </div>
          {editing && <span className="mt-1 block text-[11px] text-ink-500">type cannot be changed</span>}
        </div>

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

        {/* Step 2 — connection, shaped per kind. */}
        <fieldset className="rounded-xl border border-ink-800 bg-ink-950/40 p-3.5">
          <legend className="px-1 font-display text-[10px] font-semibold uppercase tracking-[0.12em] text-ink-400">
            Connection
          </legend>
          <div className="flex flex-col gap-3">
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
                    required={!editing}
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
              </>
            )}

            {form.kind === 'octoprint' && (
              <>
                <Field label="Base URL" hint={editing ? 'leave blank to keep current' : 'http://192.168.0.x'}>
                  <input
                    className={inputCls}
                    value={form.baseUrl}
                    onChange={(e) => set('baseUrl', e.target.value)}
                    required={!editing}
                    placeholder="http://192.168.0.22"
                  />
                </Field>
                <Field
                  label="API key"
                  hint={editing ? 'leave blank to keep current' : 'OctoPrint → Settings → Application Keys'}
                >
                  <input
                    className={inputCls}
                    value={form.apiKey}
                    onChange={(e) => set('apiKey', e.target.value)}
                    required={!editing}
                  />
                </Field>
              </>
            )}

            {form.kind === 'prusalink' && (
              <>
                <Field
                  label="Base URL"
                  hint={editing ? 'leave blank to keep current' : "the printer's IP, e.g. http://192.168.0.x"}
                >
                  <input
                    className={inputCls}
                    value={form.baseUrl}
                    onChange={(e) => set('baseUrl', e.target.value)}
                    required={!editing}
                    placeholder="http://192.168.0.40"
                  />
                </Field>
                <Field
                  label="Password / API key"
                  hint={
                    editing
                      ? 'leave blank to keep current'
                      : 'the PrusaLink password (a.k.a. API key) — printer: Settings → Network → PrusaLink. The login name is always "maker".'
                  }
                >
                  <input
                    className={inputCls}
                    value={form.apiKey}
                    onChange={(e) => set('apiKey', e.target.value)}
                    required={!editing}
                  />
                </Field>
              </>
            )}

            {form.kind === 'bambu' && (
              <div className="grid grid-cols-2 gap-3">
                <Field label="IP" hint={editing ? 'leave blank to keep' : undefined}>
                  <input
                    className={inputCls}
                    value={form.ip}
                    onChange={(e) => set('ip', e.target.value)}
                    required={!editing}
                    placeholder="192.168.0.23"
                  />
                </Field>
                <Field label="Serial" hint={editing ? 'leave blank to keep' : undefined}>
                  <input
                    className={inputCls}
                    value={form.serial}
                    onChange={(e) => set('serial', e.target.value)}
                    required={!editing}
                  />
                </Field>
                <Field label="Access code" hint={editing ? 'leave blank to keep' : 'LAN Access Code (printer screen)'}>
                  <input
                    className={inputCls}
                    value={form.accessCode}
                    onChange={(e) => set('accessCode', e.target.value)}
                    required={!editing}
                  />
                </Field>
                <Field label="Model" hint="x1 | x1c | p1s | p1p | a1 | a1mini">
                  <input
                    className={inputCls}
                    value={form.model}
                    onChange={(e) => set('model', e.target.value)}
                    placeholder="x1c"
                  />
                </Field>
              </div>
            )}
          </div>
        </fieldset>

        {/* Camera + Web UI apply to every printer kind: attach ANY camera you've
            added (printer-mounted or a standalone IP/USB-restreamed cam), not just a
            printer-attached one. */}
        <div className="grid grid-cols-2 gap-3">
          <Field label="Camera" hint="optional; any camera you've set up">
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

        {supportsMacros && (
          <Field
            label="Macro buttons"
            hint={
              form.kind === 'moonraker'
                ? 'comma-separated Klipper macros to show as buttons (e.g. T0,T1,T2,T3,M600). Blank = a safe default set.'
                : 'comma-separated gcode/macros to show as buttons (e.g. M600,M104 S0). Blank = a safe default set.'
            }
          >
            <input
              className={inputCls}
              value={macrosText}
              onChange={(e) => setMacrosText(e.target.value)}
              placeholder="M600,T0,T1"
            />
          </Field>
        )}

        <Actions onCancel={onClose} submitLabel={editing ? 'Save' : 'Add printer'} />
      </form>
    </Modal>
  )
}
