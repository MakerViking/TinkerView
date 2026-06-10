import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { cn } from '@/lib/utils'
import { IconDots } from '@/components/icons'

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
    setPos({ top: r.bottom + 6, right: window.innerWidth - r.right })
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
        aria-label="Manage device"
        aria-haspopup="menu"
        aria-expanded={open}
        title="Manage"
        className="grid h-6 w-6 place-items-center rounded-md bg-white/10 text-ink-100 backdrop-blur-sm transition-colors hover:bg-white/20"
      >
        <IconDots className="text-[14px]" />
      </button>
      {open &&
        pos &&
        createPortal(
          <div
            ref={menuRef}
            role="menu"
            style={{ position: 'fixed', top: pos.top, right: pos.right, zIndex: 60 }}
            className="min-w-[12rem] rounded-xl border border-ink-700 bg-ink-850 py-1.5 shadow-pop animate-pop"
          >
            {actions.map((a, i) => (
              <button
                key={i}
                role="menuitem"
                onClick={() => {
                  setOpen(false)
                  a.onClick()
                }}
                className={cn(
                  'block w-full px-3.5 py-1.5 text-left text-sm transition-colors',
                  a.danger
                    ? 'text-rose-300 hover:bg-rec/10'
                    : 'text-ink-100 hover:bg-ink-800',
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
