import { type ReactNode, useEffect, useRef } from 'react'
import { cn } from '@/lib/utils'
import { IconX } from '@/components/icons'

// Modal is a centered dialog with a blurred backdrop. Escape / backdrop click
// close it; focus moves into the panel on open and is restored on close.
export function Modal({
  title,
  onClose,
  children,
  size = 'md',
}: {
  title: string
  onClose: () => void
  children: ReactNode
  size?: 'md' | 'lg' | 'xl'
}) {
  const panelRef = useRef<HTMLDivElement>(null)
  // Callers pass inline closures; keep the latest in a ref so the effect below
  // runs once per open (a re-run would yank focus out of the form being edited).
  const onCloseRef = useRef(onClose)
  // The opener, captured during the first render: by effect time a child's
  // autoFocus has already moved focus.
  const openerRef = useRef(document.activeElement as HTMLElement | null)
  onCloseRef.current = onClose

  useEffect(() => {
    const panel = panelRef.current
    const opener = openerRef.current
    // Respect a child's autoFocus (it already ran); otherwise focus the panel.
    if (panel && !panel.contains(document.activeElement)) panel.focus()
    const onKey = (e: KeyboardEvent) => {
      // With stacked dialogs only the topmost (last in the DOM) reacts.
      const open = document.querySelectorAll('[role="dialog"][aria-modal="true"]')
      if (open[open.length - 1] !== panel) return
      if (e.key === 'Escape') {
        onCloseRef.current()
        return
      }
      if (e.key !== 'Tab' || !panel) return
      // Focus trap: wrap Tab / Shift+Tab inside the dialog.
      const items = Array.from(
        panel.querySelectorAll<HTMLElement>(
          'a[href], button, input, select, textarea, [tabindex]:not([tabindex="-1"])',
        ),
      ).filter((el) => !(el as HTMLButtonElement).disabled && el.offsetParent !== null)
      if (items.length === 0) {
        e.preventDefault()
        panel.focus()
        return
      }
      const first = items[0]
      const last = items[items.length - 1]
      const active = document.activeElement
      if (e.shiftKey && (active === first || active === panel || !panel.contains(active))) {
        e.preventDefault()
        last.focus()
      } else if (!e.shiftKey && (active === last || !panel.contains(active))) {
        e.preventDefault()
        first.focus()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('keydown', onKey)
      opener?.focus?.()
    }
  }, [])

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-ink-1000/70 p-4 backdrop-blur-sm animate-fade sm:items-center"
      onClick={onClose}
    >
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        tabIndex={-1}
        className={cn(
          'my-8 w-full rounded-2xl border border-ink-700 bg-ink-900 shadow-pop animate-pop outline-none',
          size === 'xl' ? 'max-w-3xl' : size === 'lg' ? 'max-w-xl' : 'max-w-md',
        )}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between gap-4 border-b border-ink-800 px-5 py-3.5">
          <h2 className="font-display font-semibold tracking-wide text-ink-50">{title}</h2>
          <button
            type="button"
            onClick={onClose}
            className="-mr-1 grid h-7 w-7 place-items-center rounded-md text-ink-400 transition-colors hover:bg-ink-800 hover:text-ink-100"
            aria-label="Close dialog"
          >
            <IconX className="text-[15px]" />
          </button>
        </div>
        <div className="p-5">{children}</div>
      </div>
    </div>
  )
}
