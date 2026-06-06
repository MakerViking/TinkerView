import { type ReactNode } from 'react'

// Shared form primitives for the device add/edit modals.

export const inputCls =
  'mt-1 w-full rounded-md bg-slate-800 border border-slate-700 px-2.5 py-1.5 text-sm text-slate-100 placeholder:text-slate-500 focus:outline-none focus:border-sky-500'

export function Field({
  label,
  hint,
  children,
}: {
  label: string
  hint?: ReactNode
  children: ReactNode
}) {
  return (
    <label className="block">
      <span className="text-xs text-slate-400">{label}</span>
      {children}
      {hint && <span className="mt-0.5 block text-[11px] text-slate-500">{hint}</span>}
    </label>
  )
}

export function Actions({
  onCancel,
  submitLabel,
  busy,
  danger,
}: {
  onCancel: () => void
  submitLabel: string
  busy?: boolean
  danger?: boolean
}) {
  return (
    <div className="mt-5 flex justify-end gap-2">
      <button
        type="button"
        onClick={onCancel}
        className="rounded-md px-3 py-1.5 text-sm text-slate-300 hover:bg-slate-800"
      >
        Cancel
      </button>
      <button
        type="submit"
        disabled={busy}
        className={
          'rounded-md px-3 py-1.5 text-sm font-medium text-white disabled:opacity-50 ' +
          (danger ? 'bg-rose-600 hover:bg-rose-500' : 'bg-sky-600 hover:bg-sky-500')
        }
      >
        {submitLabel}
      </button>
    </div>
  )
}
