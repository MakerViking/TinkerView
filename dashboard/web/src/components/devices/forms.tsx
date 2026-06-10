import { type ReactNode } from 'react'
import { Button } from '@/components/ui'

// Shared form primitives for the device add/edit modals.

export const inputCls =
  'mt-1 w-full rounded-lg border border-ink-700 bg-ink-950/70 px-3 py-2 text-sm text-ink-50 ' +
  'placeholder:text-ink-500 transition-colors hover:border-ink-600 ' +
  'focus:outline-none focus:border-brand-500 focus:ring-2 focus:ring-brand-500/20 ' +
  'disabled:opacity-50 disabled:cursor-not-allowed'

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
      <span className="font-display text-[11px] font-medium uppercase tracking-[0.08em] text-ink-400">
        {label}
      </span>
      {children}
      {hint && <span className="mt-1 block text-[11px] leading-snug text-ink-500">{hint}</span>}
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
      <Button type="button" variant="ghost" onClick={onCancel}>
        Cancel
      </Button>
      <Button type="submit" variant={danger ? 'danger' : 'primary'} disabled={busy}>
        {submitLabel}
      </Button>
    </div>
  )
}
