import { useEffect, useState, type ComponentType, type ReactNode, type SVGProps } from 'react'
import { ConnectionStatusBadge } from './ConnectionStatusBadge'
import { Mark } from './Logo'
import { useLive } from '@/store/live'
import { cn } from '@/lib/utils'
import {
  IconBell,
  IconCamera,
  IconChip,
  IconFilm,
  IconPrinter3d,
  IconScanFace,
  IconServer,
  IconX,
} from './icons'

export type Tab =
  | 'cameras'
  | 'recordings'
  | 'printers'
  | 'recognition'
  | 'detection'
  | 'health'
  | 'alerts'

interface TabDef {
  id: Tab
  label: string
  short?: string // bottom-nav label on phones
  icon: ComponentType<SVGProps<SVGSVGElement>>
  group: number // visual grouping in the desktop nav
}

const TABS: TabDef[] = [
  { id: 'cameras', label: 'Cameras', icon: IconCamera, group: 0 },
  { id: 'recordings', label: 'Recordings', short: 'Records', icon: IconFilm, group: 0 },
  { id: 'printers', label: 'Printers', icon: IconPrinter3d, group: 1 },
  { id: 'recognition', label: 'Recognition', short: 'Recogn.', icon: IconScanFace, group: 2 },
  { id: 'detection', label: 'Detection', short: 'Detect', icon: IconChip, group: 2 },
  { id: 'health', label: 'Server', icon: IconServer, group: 3 },
  { id: 'alerts', label: 'Notifications', short: 'Alerts', icon: IconBell, group: 3 },
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
      <header className="sticky top-0 z-20 border-b border-ink-800/80 bg-ink-950/80 backdrop-blur-md">
        <div className="flex h-14 items-center gap-4 px-4">
          <span className="flex items-center gap-2.5 whitespace-nowrap">
            <Mark className="h-7 w-7 shrink-0" />
            <span className="flex flex-col leading-none">
              <span className="flex items-baseline gap-1.5">
                <span className="font-display text-[15px] font-bold tracking-wide text-ink-50">
                  TinkerView
                </span>
                {version && (
                  <span
                    className="hidden font-mono text-[10px] text-ink-500 sm:inline"
                    title="running version"
                  >
                    {/^\d/.test(version) ? `v${version}` : version}
                  </span>
                )}
              </span>
              <span className="mt-0.5 font-display text-[9px] font-medium uppercase tracking-[0.18em] text-ink-500">
                Cameras + 3D printers
              </span>
            </span>
          </span>

          {/* Desktop nav: grouped — NVR · workshop · AI · system. */}
          <nav aria-label="Sections" className="hidden items-center md:flex">
            {TABS.map((t, i) => {
              const Icon = t.icon
              const newGroup = i > 0 && TABS[i - 1].group !== t.group
              return (
                <span key={t.id} className="flex items-center">
                  {newGroup && <span aria-hidden="true" className="mx-1.5 h-4 w-px bg-ink-800" />}
                  <button
                    onClick={() => onTab(t.id)}
                    aria-current={tab === t.id ? 'page' : undefined}
                    title={t.label}
                    className={cn(
                      'flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 font-display text-[13px] font-medium tracking-wide transition-colors',
                      tab === t.id
                        ? 'bg-brand-500/12 text-brand-300 shadow-[inset_0_0_0_1px_rgb(56_189_248/0.2)]'
                        : 'text-ink-400 hover:bg-ink-850 hover:text-ink-100',
                    )}
                  >
                    <Icon className="text-[15px]" />
                    <span className="hidden lg:inline">{t.label}</span>
                  </button>
                </span>
              )
            })}
          </nav>

          <div className="ml-auto flex items-center gap-2">
            <NotificationBell tab={tab} onTab={onTab} />
            <ConnectionStatusBadge />
          </div>
        </div>
      </header>

      <DataDirBanner />
      <main className="flex-1 p-4 pb-24 md:pb-6">{children}</main>

      {/* Phone: bottom tab bar (thumb-reachable). */}
      <nav
        aria-label="Sections"
        className="fixed inset-x-0 bottom-0 z-20 border-t border-ink-800/80 bg-ink-950/90 backdrop-blur-md pb-[env(safe-area-inset-bottom)] md:hidden"
      >
        <div className="grid grid-cols-7">
          {TABS.map((t) => {
            const Icon = t.icon
            const active = tab === t.id
            return (
              <button
                key={t.id}
                onClick={() => onTab(t.id)}
                aria-current={active ? 'page' : undefined}
                className={cn(
                  'flex flex-col items-center gap-1 py-2 transition-colors',
                  active ? 'text-brand-300' : 'text-ink-500 active:text-ink-300',
                )}
              >
                <span className="relative">
                  <Icon className="text-[19px]" />
                  {t.id === 'alerts' && <UnreadDot tab={tab} />}
                </span>
                <span className="font-display text-[9px] font-medium uppercase tracking-wide">
                  {t.short ?? t.label}
                </span>
                <span
                  aria-hidden="true"
                  className={cn(
                    'h-0.5 w-6 rounded-full transition-colors',
                    active ? 'bg-brand-400' : 'bg-transparent',
                  )}
                />
              </button>
            )
          })}
        </div>
      </nav>

      <NoticeToast />
    </div>
  )
}

// UnreadDot marks the bottom-nav Alerts item when new detections arrived.
function UnreadDot({ tab }: { tab: Tab }) {
  const count = useLive((s) => s.alertSeq)
  const [seen, setSeen] = useState(0)
  useEffect(() => {
    if (tab === 'alerts') setSeen(count)
  }, [tab, count])
  if (count - seen <= 0) return null
  return (
    <span className="absolute -right-1 -top-0.5 h-2 w-2 rounded-full bg-rec ring-2 ring-ink-950" />
  )
}

// NotificationBell opens the Notifications view and badges how many detections
// arrived since it was last viewed.
function NotificationBell({ tab, onTab }: { tab: Tab; onTab: (t: Tab) => void }) {
  const count = useLive((s) => s.alertSeq)
  const [seen, setSeen] = useState(0)
  useEffect(() => {
    if (tab === 'alerts') setSeen(count)
  }, [tab, count])
  const unread = Math.max(0, count - seen)
  return (
    <button
      onClick={() => onTab('alerts')}
      aria-label={unread > 0 ? `Notifications (${unread} unread)` : 'Notifications'}
      title="Notifications"
      className={cn(
        'relative grid h-8 w-8 place-items-center rounded-lg transition-colors',
        tab === 'alerts'
          ? 'bg-brand-500/12 text-brand-300'
          : 'text-ink-400 hover:bg-ink-850 hover:text-ink-100',
      )}
    >
      <IconBell className="text-[17px]" />
      {unread > 0 && (
        <span className="absolute -right-1 -top-1 min-w-[1rem] rounded-full bg-rec px-1 font-mono text-[10px] font-semibold leading-4 text-white">
          {unread > 99 ? '99+' : unread}
        </span>
      )}
    </button>
  )
}

// DataDirBanner is a persistent warning shown when the dashboard's writable data
// directory isn't writable — the root cause of "adding a printer/camera didn't
// save." Common on container hosts (e.g. CasaOS) where the bind-mounted data dir is
// root-owned; the fix is to make it writable by the container's uid (65532).
function DataDirBanner() {
  const dataWritable = useLive((s) => s.dataWritable)
  if (dataWritable) return null
  return (
    <div
      role="alert"
      className="border-b border-amber-500/40 bg-amber-950/80 px-4 py-2 text-center text-[13px] text-amber-100"
    >
      <span className="font-semibold">Data directory not writable.</span> Adding or editing printers
      and cameras won’t save. Make the dashboard’s <code className="font-mono">/data</code> volume
      writable by the container user (uid 65532), e.g.{' '}
      <code className="font-mono">chown -R 65532:65532 &lt;data dir&gt;</code>.
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
    // Errors/warnings stay until dismissed — a failed add/edit must not silently
    // vanish after 5s (that's how "it didn't work while saving" goes unnoticed).
    // Only transient info auto-dismisses.
    if (notice.level !== 'warn' && notice.level !== 'error') {
      const t = window.setTimeout(() => setVisible(false), 5000)
      return () => window.clearTimeout(t)
    }
  }, [notice?.ts])

  if (!notice || !visible) return null
  const tone =
    notice.level === 'error'
      ? 'border-rose-500/40 bg-rose-950/95 text-rose-100'
      : notice.level === 'warn'
        ? 'border-amber-500/40 bg-amber-950/95 text-amber-100'
        : 'border-ink-700 bg-ink-850/95 text-ink-100'
  const barTone =
    notice.level === 'error' ? 'bg-rec' : notice.level === 'warn' ? 'bg-amber-400' : 'bg-brand-400'
  return (
    <div
      role="status"
      className={cn(
        'fixed bottom-20 right-4 z-50 flex max-w-sm items-start gap-3 overflow-hidden rounded-xl border py-3 pl-4 pr-2 text-sm shadow-pop backdrop-blur animate-rise md:bottom-4',
        tone,
      )}
    >
      <span aria-hidden="true" className={cn('absolute inset-y-0 left-0 w-1', barTone)} />
      <span className="leading-snug">{notice.msg}</span>
      <button
        onClick={() => setVisible(false)}
        aria-label="Dismiss notification"
        className="grid h-6 w-6 shrink-0 place-items-center rounded-md opacity-60 transition-opacity hover:opacity-100"
      >
        <IconX className="text-[13px]" />
      </button>
    </div>
  )
}
