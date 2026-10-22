import type { ButtonHTMLAttributes, ReactNode } from 'react'
import { cn } from '@/lib/utils'

// ── Shared UI primitives ────────────────────────────────────────────────────
// One vocabulary of buttons, badges, cards, and states used across every tab so
// the dashboard reads as a single instrument, not a pile of ad-hoc classes.

type ButtonVariant = 'primary' | 'soft' | 'ghost' | 'danger' | 'danger-soft' | 'outline'
type ButtonSize = 'xs' | 'sm' | 'md'

const VARIANT: Record<ButtonVariant, string> = {
  primary:
    'bg-brand-500 text-ink-1000 font-semibold hover:bg-brand-400 active:bg-brand-600 shadow-[0_0_0_1px_rgb(14_165_233/0.4),0_4px_16px_-6px_rgb(14_165_233/0.5)]',
  soft: 'bg-ink-800 text-ink-100 hover:bg-ink-700 active:bg-ink-800',
  ghost: 'text-ink-300 hover:text-ink-100 hover:bg-ink-800/70',
  danger: 'bg-rec/90 text-white font-semibold hover:bg-rec',
  'danger-soft': 'bg-rec/15 text-rose-300 hover:bg-rec/25',
  outline: 'border border-ink-700 text-ink-200 hover:border-ink-500 hover:text-ink-50',
}

const SIZE: Record<ButtonSize, string> = {
  xs: 'px-2 py-1 text-xs gap-1 rounded-md',
  sm: 'px-2.5 py-1.5 text-sm gap-1.5 rounded-lg',
  md: 'px-3.5 py-2 text-sm gap-2 rounded-lg',
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
  brand: 'bg-brand-500/15 text-brand-300 ring-brand-400/25',
  ok: 'bg-emerald-500/12 text-emerald-300 ring-emerald-400/25',
  warn: 'bg-amber-500/12 text-amber-300 ring-amber-400/25',
  danger: 'bg-rec/12 text-rose-300 ring-rose-400/25',
  neutral: 'bg-ink-800/80 text-ink-300 ring-ink-600/40',
  rec: 'bg-rec/15 text-rose-200 ring-rec/40',
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
        'inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 ring-1 ring-inset',
        'font-display text-[10px] font-semibold uppercase tracking-[0.08em] leading-4',
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
    <div className={cn('rounded-xl border border-ink-800 bg-ink-900/80 shadow-elev', className)}>
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
      <div className="flex items-baseline gap-2.5">
        <h1 className="font-display text-base font-semibold tracking-wide text-ink-50">{title}</h1>
        {meta && <span className="font-mono text-[11px] text-ink-400">{meta}</span>}
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
    <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-ink-700/80 px-6 py-14 text-center animate-fade">
      {icon && (
        <div className="grid h-14 w-14 place-items-center rounded-2xl border border-ink-700 bg-ink-900 text-[26px] text-ink-400">
          {icon}
        </div>
      )}
      <div className="font-display font-medium text-ink-200">{title}</div>
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
        'animate-shimmer rounded-lg bg-[linear-gradient(100deg,transparent_30%,rgb(125_211_252/0.07)_50%,transparent_70%)] bg-ink-850 bg-[length:200%_100%]',
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
    info: 'border-ink-700 bg-ink-900/80 text-ink-200',
    warn: 'border-amber-500/30 bg-amber-950/40 text-amber-200',
    error: 'border-rose-500/30 bg-rose-950/40 text-rose-200',
    ok: 'border-emerald-500/30 bg-emerald-950/40 text-emerald-200',
  }[tone]
  return <div className={cn('rounded-xl border p-3 text-sm leading-relaxed', cls, className)}>{children}</div>
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
        on ? 'bg-brand-500' : 'bg-ink-700',
        disabled && 'opacity-40 cursor-not-allowed',
      )}
    >
      <span
        className={cn(
          'rounded-full bg-white shadow transition-transform',
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
      className={cn('inline-flex gap-0.5 rounded-lg border border-ink-800 bg-ink-950/60 p-0.5', className)}
    >
      {options.map((o) => (
        <button
          key={o.value}
          role="tab"
          aria-selected={value === o.value}
          onClick={() => onChange(o.value)}
          className={cn(
            'rounded-md px-3 py-1 font-display text-xs font-medium tracking-wide transition-colors',
            value === o.value
              ? 'bg-ink-800 text-brand-300 shadow-[inset_0_0_0_1px_rgb(56_189_248/0.18)]'
              : 'text-ink-400 hover:text-ink-200',
          )}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}
