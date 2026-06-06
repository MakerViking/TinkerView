import { useEffect, useRef, useState } from 'react'
import { cn } from '@/lib/utils'

export interface MenuAction {
  label: string
  danger?: boolean
  onClick: () => void
}

// DeviceMenu is a small "⋯" dropdown of actions (edit/remove/promote…). Closes on
// outside click or Escape.
export function DeviceMenu({ actions, className }: { actions: MenuAction[]; className?: string }) {
  const [open, setOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onDoc = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', onDoc)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDoc)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  if (!actions.length) return null

  return (
    <div ref={ref} className={cn('relative', className)}>
      <button
        onClick={() => setOpen((o) => !o)}
        title="Manage"
        className="rounded px-1.5 py-0.5 text-sm leading-none text-slate-300 hover:bg-white/20"
      >
        ⋯
      </button>
      {open && (
        <div className="absolute right-0 z-30 mt-1 min-w-[10rem] rounded-md border border-slate-700 bg-slate-900 py-1 shadow-lg">
          {actions.map((a, i) => (
            <button
              key={i}
              onClick={() => {
                setOpen(false)
                a.onClick()
              }}
              className={cn(
                'block w-full px-3 py-1.5 text-left text-sm hover:bg-slate-800',
                a.danger ? 'text-rose-300' : 'text-slate-200',
              )}
            >
              {a.label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
