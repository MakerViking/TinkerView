import { useEffect, useState } from 'react'
import { Modal } from '@/components/Modal'
import { useLive } from '@/store/live'
import { useShallow } from 'zustand/react/shallow'
import type { PrinterInput } from '@/types/ws'
import type { PrinterDetail } from '@/types/devices'
import { apiGet, apiSend } from '@/lib/api'
import { Actions, Field, FormError, FormSkeleton, inputCls } from './forms'
import { CameraSelect, ConnectionFields, PRINTER_KINDS, kindSupportsMacros, macrosHint, parseMacros } from './PrinterFields'

// EditPrinterModal edits an existing printer. Type and ID are fixed. The stored
// values are fetched on open and prefilled; secrets (API key / access code) are
// never sent to the browser, so those inputs stay blank and blank means "keep".
// Submits via REST and stays open with the error inline on failure.
export function EditPrinterModal({ id, onClose }: { id: string; onClose: () => void }) {
  const cameras = useLive(useShallow((s) => Object.keys(s.cameras).sort()))
  const [detail, setDetail] = useState<PrinterDetail | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [form, setForm] = useState<PrinterInput | null>(null)
  const [macrosText, setMacrosText] = useState('')
  // Snapshot of the prefilled values, to know when the form is dirty.
  const [initial, setInitial] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    void apiGet<PrinterDetail>(`/api/printers/${encodeURIComponent(id)}`).then((r) => {
      if (cancelled) return
      if (r.error !== undefined) {
        setLoadError(r.error)
        return
      }
      const d = r.data
      const f: PrinterInput = {
        id: d.id,
        kind: d.kind,
        name: d.name,
        baseUrl: d.baseUrl ?? '',
        apiKey: '',
        ip: d.ip ?? '',
        serial: d.serial ?? '',
        accessCode: '',
        model: d.model ?? '',
        camera: d.camera ?? '',
        uiUrl: d.uiUrl ?? '',
      }
      const m = (d.macros ?? []).join(', ')
      setDetail(d)
      setForm(f)
      setMacrosText(m)
      setInitial(JSON.stringify([f, m]))
    })
    return () => {
      cancelled = true
    }
  }, [id])

  const set = (k: keyof PrinterInput, v: string) => setForm((f) => (f ? { ...f, [k]: v } : f))

  const kindInfo = detail && PRINTER_KINDS.find((k) => k.value === detail.kind)
  const supportsMacros = !!form && kindSupportsMacros(form.kind)
  const dirty = !!form && JSON.stringify([form, macrosText]) !== initial
  let valid = false
  if (form) {
    valid = form.name.trim() !== ''
    if (form.kind === 'bambu') valid &&= !!form.ip?.trim() && !!form.serial?.trim()
    else valid &&= !!form.baseUrl?.trim()
  }

  const submit = async () => {
    if (!form || busy || !dirty || !valid) return
    const printer: PrinterInput = { ...form }
    if (supportsMacros) printer.macros = parseMacros(macrosText)
    setBusy(true)
    setError(null)
    const err = await apiSend('PUT', `/api/printers/${encodeURIComponent(id)}`, printer)
    if (err) {
      setError(err)
      setBusy(false)
      return
    }
    onClose()
  }

  return (
    <Modal title={detail ? `Edit ${detail.name}` : 'Edit printer'} onClose={onClose}>
      {loadError ? (
        <div className="flex flex-col gap-4">
          <FormError message={`Couldn't load printer: ${loadError}`} />
          <Actions onCancel={onClose} submitLabel="Save" disabled />
        </div>
      ) : !detail || !form ? (
        <FormSkeleton rows={5} />
      ) : (
        <form
          onSubmit={(e) => {
            e.preventDefault()
            void submit()
          }}
          className="flex flex-col gap-4"
        >
          <div className="grid grid-cols-2 gap-3">
            <Field label="Type" hint="cannot be changed">
              <input className={inputCls} value={kindInfo ? `${kindInfo.label} (${kindInfo.sub})` : detail.kind} disabled />
            </Field>
            <Field label="ID" hint="cannot be changed">
              <input className={inputCls} value={detail.id} disabled />
            </Field>
          </div>

          <Field label="Name">
            <input
              className={inputCls}
              value={form.name}
              onChange={(e) => set('name', e.target.value)}
              required
              placeholder="Voron 2.4"
            />
          </Field>

          <fieldset className="rounded-lg border border-ink-850 bg-ink-900 p-4">
            <legend className="px-1 text-[13px] font-medium text-ink-300">
              Connection
            </legend>
            <ConnectionFields
              form={form}
              set={set}
              editing
              hasKey={detail.hasApiKey}
              hasCode={detail.hasAccessCode}
            />
          </fieldset>

          <div className="grid grid-cols-2 gap-3">
            <CameraSelect value={form.camera ?? ''} onChange={(v) => set('camera', v)} cameras={cameras} />
            <Field label="Web UI URL" hint="optional; blank = use the base URL">
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
          <Actions onCancel={onClose} busy={busy} disabled={!dirty || !valid} submitLabel="Save" />
        </form>
      )}
    </Modal>
  )
}
