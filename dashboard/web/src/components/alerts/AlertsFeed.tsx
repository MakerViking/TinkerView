import { useLive } from '@/store/live'
import type { Alert } from '@/types/ws'
import { cn } from '@/lib/utils'

const KIND_ICON: Record<Alert['kind'], string> = {
  person: '🚶',
  motion: '🌀',
  camera_offline: '📵',
  print_done: '✅',
  print_failed: '❌',
  frigate_down: '⚠️',
}

export function AlertsFeed() {
  const alerts = useLive((s) => s.alerts)
  if (!alerts.length) {
    return <div className="text-slate-400 p-8 text-center">No alerts yet.</div>
  }
  return (
    <div className="flex flex-col gap-2 max-w-2xl">
      {alerts.map((a) => (
        <AlertCard key={a.id} a={a} />
      ))}
    </div>
  )
}

function AlertCard({ a }: { a: Alert }) {
  return (
    <div className="flex items-center gap-3 rounded-lg border border-slate-800 bg-slate-900/60 p-3">
      <span className="text-xl">{KIND_ICON[a.kind] ?? '🔔'}</span>
      {a.thumbUrl && (
        <img src={a.thumbUrl} alt="" className="w-16 h-10 object-cover rounded bg-black" />
      )}
      <div className="min-w-0">
        <div className="text-sm font-medium truncate">{a.title}</div>
        <div className="text-xs text-slate-400 truncate">{a.message}</div>
      </div>
      <time className="ml-auto text-xs text-slate-500 whitespace-nowrap">
        {new Date(a.ts).toLocaleTimeString()}
      </time>
      {a.clipUrl && (
        <a
          href={a.clipUrl}
          target="_blank"
          rel="noreferrer"
          className={cn('text-xs px-2 py-1 rounded bg-slate-700 text-slate-200 hover:bg-slate-600')}
        >
          Clip
        </a>
      )}
    </div>
  )
}
