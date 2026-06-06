import { fmtDuration } from '@/lib/utils'
import { cn } from '@/lib/utils'
import { useMoonraker } from './useMoonraker'

interface Job {
  status: string
  print_duration: number
  filename: string
  filament_used: number
}
interface Totals {
  result?: { job_totals?: { total_jobs?: number; total_time?: number; total_filament_used?: number } }
}

const STATUS_CLS: Record<string, string> = {
  completed: 'text-emerald-400',
  cancelled: 'text-amber-400',
  error: 'text-rose-400',
  klippy_shutdown: 'text-rose-400',
}

// HistoryPanel shows recent jobs + lifetime totals from Moonraker's history API.
export function HistoryPanel({ id }: { id: string }) {
  const jobs = useMoonraker<{ result: { jobs: Job[] } }>(id, '/server/history/list?limit=25&order=desc')
  const totals = useMoonraker<Totals>(id, '/server/history/totals')
  const list = jobs.data?.result?.jobs ?? []
  const t = totals.data?.result?.job_totals

  return (
    <div className="flex flex-col gap-2">
      {t && (
        <div className="text-[11px] text-slate-400">
          Lifetime: {t.total_jobs ?? 0} jobs · {Math.round((t.total_time ?? 0) / 3600)} h ·{' '}
          {((t.total_filament_used ?? 0) / 1000).toFixed(0)} m filament
        </div>
      )}
      {jobs.loading && !jobs.data && <div className="text-xs text-slate-500">Loading history…</div>}
      {jobs.error && <div className="text-xs text-rose-400">Couldn’t load history: {jobs.error}</div>}
      <div className="flex flex-col divide-y divide-slate-800">
        {list.map((j, i) => (
          <div key={i} className="flex items-center gap-2 py-1.5 text-xs">
            <span className={cn('w-16 shrink-0 capitalize', STATUS_CLS[j.status] ?? 'text-slate-400')}>
              {j.status}
            </span>
            <span className="min-w-0 flex-1 truncate text-slate-300">{j.filename.split('/').pop()}</span>
            <span className="shrink-0 text-slate-500">{fmtDuration(j.print_duration)}</span>
          </div>
        ))}
        {jobs.data && !list.length && <div className="text-xs text-slate-500">No history yet.</div>}
      </div>
    </div>
  )
}
