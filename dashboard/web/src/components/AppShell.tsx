import { useEffect, useState, type ReactNode } from 'react'
import { ConnectionStatusBadge } from './ConnectionStatusBadge'
import { useLive } from '@/store/live'
import { cn } from '@/lib/utils'

export type Tab = 'cameras' | 'printers' | 'health' | 'alerts'

const TABS: { id: Tab; label: string }[] = [
  { id: 'cameras', label: 'Cameras' },
  { id: 'printers', label: 'Printers' },
  { id: 'health', label: 'Server' },
  { id: 'alerts', label: 'Alerts' },
]

export function AppShell({
  tab,
  onTab,
  children,
}: {
  tab: Tab
  onTab: (t: Tab) => void
  children: ReactNode
}) {
  const [version, setVersion] = useState('')
  useEffect(() => {
    fetch('/api/version')
      .then((r) => (r.ok ? r.json() : null))
      .then((d) => d?.version && setVersion(d.version))
      .catch(() => {})
  }, [])
  return (
    <div className="min-h-full flex flex-col">
      <header className="flex items-center gap-4 px-4 h-14 border-b border-slate-800 bg-slate-900/70 backdrop-blur sticky top-0 z-20">
        <span className="flex items-baseline gap-1.5 whitespace-nowrap">
          <span className="font-semibold tracking-tight">TinkerView</span>
          <span className="text-[11px] font-normal text-slate-500">by TinkerAtlas</span>
          {version && (
            <span className="hidden text-[11px] text-slate-600 sm:inline" title="running version">
              {/^\d/.test(version) ? `v${version}` : version}
            </span>
          )}
        </span>
        <nav className="flex gap-1">
          {TABS.map((t) => (
            <button
              key={t.id}
              onClick={() => onTab(t.id)}
              className={cn(
                'px-3 py-1.5 rounded-md text-sm transition',
                tab === t.id
                  ? 'bg-sky-500/20 text-sky-300'
                  : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800',
              )}
            >
              {t.label}
            </button>
          ))}
        </nav>
        <div className="ml-auto">
          <ConnectionStatusBadge />
        </div>
      </header>
      <main className="flex-1 p-4">{children}</main>
      <NoticeToast />
    </div>
  )
}

// NoticeToast briefly surfaces backend notices (command failures, warnings) pushed
// over the WS, so errors are visible in the UI and not just in the server log.
function NoticeToast() {
  const notice = useLive((s) => s.lastNotice)
  const [visible, setVisible] = useState(false)

  useEffect(() => {
    if (!notice) return
    setVisible(true)
    const t = window.setTimeout(() => setVisible(false), 5000)
    return () => window.clearTimeout(t)
  }, [notice?.ts])

  if (!notice || !visible) return null
  const color =
    notice.level === 'error'
      ? 'border-rose-700 bg-rose-950/90 text-rose-200'
      : notice.level === 'warn'
        ? 'border-amber-700 bg-amber-950/90 text-amber-200'
        : 'border-slate-700 bg-slate-900/95 text-slate-200'
  return (
    <div
      className={cn(
        'fixed bottom-4 right-4 z-50 max-w-sm px-4 py-2.5 rounded-lg border shadow-lg text-sm',
        color,
      )}
    >
      {notice.msg}
    </div>
  )
}
