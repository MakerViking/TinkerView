import { useState } from 'react'
import { Modal } from '@/components/Modal'
import { useLive } from '@/store/live'
import { useShallow } from 'zustand/react/shallow'
import type { PrinterInput } from '@/types/ws'
import { apiSend } from '@/lib/api'
import { Actions, Field, FormError, inputCls } from './forms'
import { CameraSelect, ConnectionFields, PRINTER_KINDS, kindSupportsMacros, macrosHint, parseMacros } from './PrinterFields'
import { cn } from '@/lib/utils'

// AddPrinterModal adds a new printer. Editing an existing one lives in
// EditPrinterModal. The add is a REST POST so a rejection is shown inline and the
// typed input is kept.
export function AddPrinterModal({ onClose }: { onClose: () => void }) {
  const cameras = useLive(useShallow((s) => Object.keys(s.cameras).sort()))

  const [form, setForm] = useState<PrinterInput>({
    id: '',
    kind: 'moonraker',
    name: '',
    baseUrl: '',
    apiKey: '',
    ip: '',
    serial: '',
    accessCode: '',
    model: '',
    camera: '',
    uiUrl: '',
  })
  const set = (k: keyof PrinterInput, v: string) => setForm((f) => ({ ...f, [k]: v }))
  // Curated macro allowlist as comma-separated text (e.g. "T0,T1,T2,T3,M600").
  const [macrosText, setMacrosText] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const supportsMacros = kindSupportsMacros(form.kind)

  const submit = async () => {
    if (busy) return
    const printer: PrinterInput = { ...form }
    if (supportsMacros) printer.macros = parseMacros(macrosText)
    setBusy(true)
    setError(null)
    const err = await apiSend('POST', '/api/printers', printer)
    if (err) {
      setError(err)
      setBusy(false)
      return
    }
    onClose()
  }

  return (
    <Modal title="Add printer" onClose={onClose}>
      <form
        onSubmit={(e) => {
          e.preventDefault()
          void submit()
        }}
        className="flex flex-col gap-4"
      >
        {/* Step 1 — what kind of printer? Card picker instead of a bare select. */}
        <div>
          <span className="text-[13px] font-medium text-ink-200">
            Type
          </span>
          <div role="radiogroup" aria-label="Printer type" className="mt-1.5 grid grid-cols-2 gap-2 sm:grid-cols-4">
            {PRINTER_KINDS.map((k) => {
              const active = form.kind === k.value
              return (
                <button
                  type="button"
                  role="radio"
                  aria-checked={active}
                  key={k.value}
                  onClick={() => set('kind', k.value)}
                  className={cn(
                    'flex flex-col items-start rounded-md border px-3 py-2.5 text-left transition-colors',
                    active
                      ? 'border-brand-500/60 bg-brand-500/10'
                      : 'border-ink-700 bg-ink-950/40 hover:border-ink-500',
                  )}
                >
                  <span className={cn('text-sm font-semibold', active ? 'text-brand-400' : 'text-ink-100')}>
                    {k.label}
                  </span>
                  <span className="text-[12px] text-ink-500">{k.sub}</span>
                </button>
              )
            })}
          </div>
        </div>

        <div className="grid grid-cols-2 gap-3">
          <Field label="ID" hint="lowercase, e.g. voron">
            <input
              className={inputCls}
              value={form.id}
              onChange={(e) => set('id', e.target.value)}
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
        <fieldset className="rounded-lg border border-ink-850 bg-ink-900 p-4">
          <legend className="px-1 text-[13px] font-medium text-ink-300">
            Connection
          </legend>
          <ConnectionFields form={form} set={set} />
        </fieldset>

        {/* Camera + Web UI apply to every printer kind: attach ANY camera you've
            added (printer-mounted or a standalone IP/USB-restreamed cam), not just a
            printer-attached one. */}
        <div className="grid grid-cols-2 gap-3">
          <CameraSelect value={form.camera ?? ''} onChange={(v) => set('camera', v)} cameras={cameras} />
          <Field label="Web UI URL" hint="optional; defaults to base URL">
            <input
              className={inputCls}
              value={form.uiUrl ?? ''}
              onChange={(e) => set('uiUrl', e.target.value)}
              placeholder="(optional)"
            />
          </Field>
        </div>

        {supportsMacros && (
          <Field label="Macro buttons" hint={macrosHint(form.kind)}>
            <input
              className={inputCls}
              value={macrosText}
              onChange={(e) => setMacrosText(e.target.value)}
              placeholder="M600,T0,T1"
            />
          </Field>
        )}

        <FormError message={error} />
        <Actions onCancel={onClose} busy={busy} submitLabel="Add printer" />
      </form>
    </Modal>
  )
}
