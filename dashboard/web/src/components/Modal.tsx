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

  useEffect(() => {
    const prev = document.activeElement as HTMLElement | null
    panelRef.current?.focus()
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('keydown', onKey)
      prev?.focus?.()
    }
  }, [onClose])

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
