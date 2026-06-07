import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { cn } from '@/lib/utils'

export interface MenuAction {
  label: string
  danger?: boolean
  onClick: () => void
}

// DeviceMenu is a small "⋯" dropdown of actions (edit/remove/promote…). The menu
// is rendered in a portal with fixed positioning so it is NEVER clipped by an
// `overflow-hidden` ancestor (e.g. the camera tile). Closes on outside click or Escape.
export function DeviceMenu({ actions, className }: { actions: MenuAction[]; className?: string }) {
  const [open, setOpen] = useState(false)
  const btnRef = useRef<HTMLButtonElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const [pos, setPos] = useState<{ top: number; right: number } | null>(null)

  useLayoutEffect(() => {
    if (!open || !btnRef.current) return
    const r = btnRef.current.getBoundingClientRect()
    setPos({ top: r.bottom + 4, right: window.innerWidth - r.right })
  }, [open])

  useEffect(() => {
    if (!open) return
    const onDoc = (e: MouseEvent) => {
      const t = e.target as Node
      if (btnRef.current?.contains(t) || menuRef.current?.contains(t)) return
      setOpen(false)
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
    <div className={cn('relative', className)}>
      <button
        ref={btnRef}
        onClick={() => setOpen((o) => !o)}
        title="Manage"
        className="rounded px-1.5 py-0.5 text-sm leading-none text-slate-200 bg-white/10 hover:bg-white/20"
      >
        ⋯
      </button>
      {open &&
        pos &&
        createPortal(
          <div
            ref={menuRef}
            style={{ position: 'fixed', top: pos.top, right: pos.right, zIndex: 60 }}
            className="min-w-[11rem] rounded-md border border-slate-700 bg-slate-900 py-1 shadow-lg"
          >
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
          </div>,
          document.body,
        )}
    </div>
  )
}
