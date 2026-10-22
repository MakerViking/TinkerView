import { type ReactNode } from 'react'
import { Button, Skeleton } from '@/components/ui'

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
  disabled,
  danger,
}: {
  onCancel: () => void
  submitLabel: string
  busy?: boolean
  disabled?: boolean
  danger?: boolean
}) {
  return (
    <div className="mt-5 flex justify-end gap-2">
      <Button type="button" variant="ghost" onClick={onCancel}>
        Cancel
      </Button>
      <Button type="submit" variant={danger ? 'danger' : 'primary'} disabled={busy || disabled}>
        {submitLabel}
      </Button>
    </div>
  )
}

// FormError renders a server/validation failure inline, inside the open modal.
export function FormError({ message }: { message: string | null }) {
  if (!message) return null
  return (
    <div
      role="alert"
      className="rounded-lg border border-rec/40 bg-rec/10 px-3 py-2 text-[13px] leading-snug text-rose-200 break-words"
    >
      {message}
    </div>
  )
}

// FormSkeleton is the loading placeholder shown while an edit form's GET is in flight.
export function FormSkeleton({ rows = 4 }: { rows?: number }) {
  return (
    <div className="flex flex-col gap-4" aria-busy="true">
      {Array.from({ length: rows }, (_, i) => (
        <Skeleton key={i} className="h-10" />
      ))}
    </div>
  )
}
