import type { ComponentType, SVGProps } from 'react'
import { useLive } from '@/store/live'
import type { Alert } from '@/types/ws'
import { Badge, EmptyState, SectionHeader } from '@/components/ui'
import {
  IconActivity,
  IconBell,
  IconCameraOff,
  IconCheck,
  IconPerson,
  IconServer,
  IconWarning,
} from '@/components/icons'
import { cn } from '@/lib/utils'

const KIND_META: Record<Alert['kind'], { icon: ComponentType<SVGProps<SVGSVGElement>>; cls: string }> = {
  person: { icon: IconPerson, cls: 'bg-brand-500/12 text-brand-300' },
  motion: { icon: IconActivity, cls: 'bg-ink-800 text-ink-300' },
  camera_offline: { icon: IconCameraOff, cls: 'bg-amber-500/12 text-amber-300' },
  print_done: { icon: IconCheck, cls: 'bg-emerald-500/12 text-emerald-300' },
  print_failed: { icon: IconWarning, cls: 'bg-rec/12 text-rose-300' },
  frigate_down: { icon: IconServer, cls: 'bg-rec/12 text-rose-300' },
}

export function AlertsFeed() {
  const alerts = useLive((s) => s.alerts)
  return (
    <div className="flex max-w-2xl flex-col gap-3">
      <SectionHeader title="Notifications" meta={alerts.length ? `last ${alerts.length}` : undefined} />
      {alerts.length ? (
        <div className="flex flex-col gap-2">
          {alerts.map((a) => (
            <AlertCard key={a.id} a={a} />
          ))}
        </div>
      ) : (
        <EmptyState
          icon={<IconBell />}
          title="No alerts yet"
          hint="Detections, print events and service warnings show up here as they happen."
        />
      )}
    </div>
  )
}

function AlertCard({ a }: { a: Alert }) {
  const meta = KIND_META[a.kind] ?? { icon: IconBell, cls: 'bg-ink-800 text-ink-300' }
  const Icon = meta.icon
  return (
    <div className="flex items-center gap-3 rounded-xl border border-ink-800 bg-ink-900/80 p-3 shadow-elev">
      <span className={cn('grid h-9 w-9 shrink-0 place-items-center rounded-lg', meta.cls)}>
        <Icon className="text-[17px]" />
      </span>
      {a.thumbUrl && (
        <img src={a.thumbUrl} alt="" className="h-10 w-16 rounded-md border border-ink-800 bg-black object-cover" />
      )}
      <div className="min-w-0">
        <div className="flex items-center gap-1.5">
          <span className="truncate text-sm font-medium text-ink-50">{a.title}</span>
          {a.subLabel ? (
            <Badge tone="ok" className="shrink-0">
              {a.subLabel}
            </Badge>
          ) : (
            a.label === 'person' && (
              <Badge tone="warn" className="shrink-0">
                unknown
              </Badge>
            )
          )}
        </div>
        <div className="truncate text-xs text-ink-400">{a.message}</div>
        {a.zones && a.zones.length > 0 && (
          <div className="mt-1 flex flex-wrap gap-1">
            {a.zones.map((z) => (
              <span key={z} className="rounded bg-ink-800 px-1.5 font-mono text-[10px] text-ink-400">
                {z}
              </span>
            ))}
          </div>
        )}
      </div>
      <time className="ml-auto whitespace-nowrap font-mono text-[11px] text-ink-500">
        {new Date(a.ts).toLocaleTimeString()}
      </time>
      {a.clipUrl && (
        <a
          href={a.clipUrl}
          target="_blank"
          rel="noreferrer"
          className="rounded-lg bg-ink-800 px-2.5 py-1.5 text-xs text-ink-100 transition-colors hover:bg-ink-700"
        >
          Clip
        </a>
      )}
    </div>
  )
}
