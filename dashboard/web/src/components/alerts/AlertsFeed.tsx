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
  person: { icon: IconPerson, cls: 'text-ink-500' },
  motion: { icon: IconActivity, cls: 'text-ink-500' },
  camera_offline: { icon: IconCameraOff, cls: 'text-brand-300' },
  print_done: { icon: IconCheck, cls: 'text-ink-500' },
  print_failed: { icon: IconWarning, cls: 'text-rose-300' },
  frigate_down: { icon: IconServer, cls: 'text-rose-300' },
}

export function AlertsFeed() {
  const alerts = useLive((s) => s.alerts)
  // Always newest-first, regardless of the initial /api/state order or how live
  // (prepended) updates merge with it.
  const sorted = [...alerts].sort((a, b) => b.ts - a.ts)
  return (
    <div className="flex max-w-3xl flex-col gap-6">
      <SectionHeader title="Notifications" meta={alerts.length ? `last ${alerts.length}` : undefined} />
      {sorted.length ? (
        <div className="divide-y divide-ink-850 overflow-hidden rounded-lg border border-ink-850 bg-ink-900">
          {sorted.map((a) => (
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
  const meta = KIND_META[a.kind] ?? { icon: IconBell, cls: 'text-ink-500' }
  const Icon = meta.icon
  return (
    <div className="group flex items-center gap-3 px-4 py-2.5">
      {a.thumbUrl ? (
        <img src={a.thumbUrl} alt="" className="aspect-video w-20 shrink-0 rounded-md bg-black object-cover" />
      ) : (
        <Icon className={cn('shrink-0 text-[17px]', meta.cls)} />
      )}
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          {a.thumbUrl && <Icon className={cn('shrink-0 text-[14px]', meta.cls)} />}
          <span className="truncate text-sm font-medium text-ink-50">{a.title}</span>
          {a.subLabel ? (
            <Badge tone="neutral" className="shrink-0">
              {a.subLabel}
            </Badge>
          ) : (
            a.label === 'person' && (
              <Badge tone="neutral" className="shrink-0">
                unknown
              </Badge>
            )
          )}
        </div>
        <div className="truncate text-[13px] text-ink-400">{a.message}</div>
        {a.zones && a.zones.length > 0 && (
          <div className="mt-1 flex flex-wrap gap-1">
            {a.zones.map((z) => (
              <span key={z} className="rounded bg-ink-850 px-1.5 font-mono text-[11px] text-ink-400">
                {z}
              </span>
            ))}
          </div>
        )}
      </div>
      <time className="ml-auto whitespace-nowrap text-[12px] tabular-nums text-ink-500">
        {new Date(a.ts).toLocaleTimeString()}
      </time>
      {/* Fixed-width slot so timestamps line up whether or not a row has a clip. */}
      <span className="flex w-10 shrink-0 justify-end">
        {a.clipUrl && (
          <a
            href={a.clipUrl}
            target="_blank"
            rel="noreferrer"
            className="rounded-md px-2 py-1 text-xs text-ink-300 opacity-0 transition-colors hover:bg-ink-850 hover:text-ink-100 focus-visible:opacity-100 group-hover:opacity-100 group-focus-within:opacity-100 pointer-coarse:opacity-100"
          >
            Clip
          </a>
        )}
      </span>
    </div>
  )
}
