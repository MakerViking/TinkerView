import type { ButtonHTMLAttributes, ReactNode } from 'react'
import { cn } from '@/lib/utils'

// ── Shared UI primitives ────────────────────────────────────────────────────
// One vocabulary of buttons, badges, cards, and states used across every tab so
// the dashboard reads as a single instrument, not a pile of ad-hoc classes.

type ButtonVariant = 'primary' | 'soft' | 'ghost' | 'danger' | 'danger-soft' | 'outline'
type ButtonSize = 'xs' | 'sm' | 'md'

const VARIANT: Record<ButtonVariant, string> = {
  primary:
    'bg-brand-400 text-ink-1000 font-semibold hover:bg-brand-300 active:bg-brand-500',
  soft: 'bg-ink-850 text-ink-100 ring-1 ring-inset ring-ink-800 hover:bg-ink-800 active:bg-ink-850',
  ghost: 'text-ink-300 hover:text-ink-100 hover:bg-ink-850',
  danger: 'bg-rec/90 text-white font-semibold hover:bg-rec',
  'danger-soft': 'bg-rec/15 text-rose-300 hover:bg-rec/25',
  outline: 'border border-ink-700 text-ink-200 hover:border-ink-600 hover:text-ink-50',
}

const SIZE: Record<ButtonSize, string> = {
  xs: 'px-2 py-1 text-xs gap-1 rounded-md',
  sm: 'px-2.5 py-1.5 text-[13px] font-medium gap-1.5 rounded-md',
  md: 'px-3.5 py-2 text-sm font-medium gap-2 rounded-md',
}

export function Button({
  variant = 'soft',
  size = 'sm',
  className,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { variant?: ButtonVariant; size?: ButtonSize }) {
  return (
    <button
      type="button"
      {...props}
      className={cn(
        'inline-flex items-center justify-center whitespace-nowrap transition-colors select-none',
        'disabled:opacity-45 disabled:pointer-events-none',
        SIZE[size],
        VARIANT[variant],
        className,
      )}
    />
  )
}

// Badge — small status pill; tone maps to the signal palette.
export type BadgeTone = 'brand' | 'ok' | 'warn' | 'danger' | 'neutral' | 'rec'

const BADGE: Record<BadgeTone, string> = {
  brand: 'bg-brand-400/10 text-brand-300 ring-brand-400/20',
  ok: 'bg-emerald-400/10 text-emerald-300 ring-emerald-400/15',
  warn: 'bg-brand-400/10 text-brand-300 ring-brand-400/20',
  danger: 'bg-rec/10 text-rose-300 ring-rec/25',
  neutral: 'bg-ink-850 text-ink-300 ring-ink-800',
  rec: 'bg-rec/10 text-rose-200 ring-rec/30',
}

export function Badge({
  tone = 'neutral',
  className,
  children,
  title,
  dot,
  pulse,
}: {
  tone?: BadgeTone
  className?: string
  children: ReactNode
  title?: string
  dot?: boolean
  pulse?: boolean
}) {
  return (
    <span
      title={title}
      className={cn(
        'inline-flex items-center gap-1.5 rounded-md px-1.5 py-0.5 ring-1 ring-inset',
        'text-[11px] font-medium leading-4',
        BADGE[tone],
        className,
      )}
    >
      {dot && (
        <span className={cn('h-1.5 w-1.5 rounded-full bg-current', pulse && 'animate-blip')} />
      )}
      {children}
    </span>
  )
}

// Card — the standard raised panel surface.
export function Card({ className, children }: { className?: string; children: ReactNode }) {
  return (
    <div className={cn('rounded-lg border border-ink-850 bg-ink-900', className)}>
      {children}
    </div>
  )
}

// SectionHeader — per-tab heading row: title, optional meta line, actions slot.
export function SectionHeader({
  title,
  meta,
  children,
}: {
  title: string
  meta?: ReactNode
  children?: ReactNode
}) {
  return (
    <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
      <div className="flex items-baseline gap-3">
        <h1 className="text-xl font-semibold tracking-tight text-ink-50">{title}</h1>
        {meta && <span className="text-[13px] text-ink-400">{meta}</span>}
      </div>
      {children && <div className="ml-auto flex items-center gap-2">{children}</div>}
    </div>
  )
}

// EmptyState — friendly zero-data placeholder with an optional CTA.
export function EmptyState({
  icon,
  title,
  hint,
  action,
}: {
  icon?: ReactNode
  title: string
  hint?: ReactNode
  action?: ReactNode
}) {
  return (
    <div className="flex flex-col items-center gap-3 rounded-lg border border-ink-850 px-6 py-16 text-center animate-fade">
      {icon && <div className="text-[28px] text-ink-500">{icon}</div>}
      <div className="font-medium text-ink-100">{title}</div>
      {hint && <p className="max-w-sm text-sm text-ink-400">{hint}</p>}
      {action && <div className="mt-1">{action}</div>}
    </div>
  )
}

// Skeleton — shimmer placeholder for loading states.
export function Skeleton({ className }: { className?: string }) {
  return (
    <div
      aria-hidden="true"
      className={cn(
        'animate-shimmer rounded-md bg-[linear-gradient(100deg,transparent_30%,rgb(255_255_255/0.04)_50%,transparent_70%)] bg-ink-900 bg-[length:200%_100%]',
        className,
      )}
    />
  )
}

// Notice — inline callout (info / warn / error / ok).
export function Notice({
  tone = 'info',
  className,
  children,
}: {
  tone?: 'info' | 'warn' | 'error' | 'ok'
  className?: string
  children: ReactNode
}) {
  const cls = {
    info: 'border-ink-800 bg-ink-900 text-ink-200',
    warn: 'border-brand-400/25 bg-brand-400/[0.06] text-brand-200',
    error: 'border-rec/30 bg-rec/[0.07] text-rose-200',
    ok: 'border-emerald-400/15 bg-ink-900 text-ink-200',
  }[tone]
  return <div className={cn('rounded-lg border p-3 text-sm leading-relaxed', cls, className)}>{children}</div>
}

// Switch — accessible on/off toggle (role=switch) used for feature flags.
export function Switch({
  on,
  onChange,
  disabled,
  label,
  size = 'md',
}: {
  on: boolean
  onChange: (v: boolean) => void
  disabled?: boolean
  label?: string
  size?: 'sm' | 'md'
}) {
  const track = size === 'sm' ? 'h-4 w-7' : 'h-5 w-9'
  const thumb = size === 'sm' ? 'h-3 w-3' : 'h-4 w-4'
  const shift = size === 'sm' ? 'translate-x-3' : 'translate-x-4'
  return (
    <button
      type="button"
      role="switch"
      aria-checked={on}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!on)}
      className={cn(
        'relative inline-flex shrink-0 items-center rounded-full p-0.5 transition-colors',
        track,
        on ? 'bg-brand-400' : 'bg-ink-700',
        disabled && 'opacity-40 cursor-not-allowed',
      )}
    >
      <span
        className={cn(
          'rounded-full bg-ink-50 shadow-sm transition-transform',
          thumb,
          on ? shift : 'translate-x-0',
        )}
      />
    </button>
  )
}

// SegmentedControl — compact tab switcher for sub-views.
export function Segmented<T extends string>({
  value,
  onChange,
  options,
  className,
}: {
  value: T
  onChange: (v: T) => void
  options: { value: T; label: ReactNode }[]
  className?: string
}) {
  return (
    <div
      role="tablist"
      className={cn('inline-flex gap-0.5 rounded-md bg-ink-900 p-0.5 ring-1 ring-inset ring-ink-850', className)}
    >
      {options.map((o) => (
        <button
          key={o.value}
          role="tab"
          aria-selected={value === o.value}
          onClick={() => onChange(o.value)}
          className={cn(
            'rounded px-3 py-1 text-[13px] font-medium transition-colors',
            value === o.value ? 'bg-ink-800 text-ink-50' : 'text-ink-400 hover:text-ink-200',
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}
