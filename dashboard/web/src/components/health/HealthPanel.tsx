import { useState } from 'react'
import { useLive } from '@/store/live'
import { useWs } from '@/ws/WebSocketProvider'
import type { Container, DiskUsage } from '@/types/ws'
import { cn, fmtBytes, fmtDuration } from '@/lib/utils'

export function HealthPanel() {
  const containers = useLive((s) => s.containers)
  const disks = useLive((s) => s.disks)
  const frigateOnline = useLive((s) => s.frigateOnline)

  return (
    <div className="flex flex-col gap-6">
      <section>
        <h2 className="text-sm font-semibold text-slate-300 mb-2">Services</h2>
        <div className="flex items-center gap-2 mb-3 text-sm">
          <span
            className={cn(
              'px-2 py-0.5 rounded text-xs',
              frigateOnline ? 'bg-emerald-500/20 text-emerald-300' : 'bg-rose-500/20 text-rose-300',
            )}
          >
            Frigate {frigateOnline ? 'online' : 'offline'}
          </span>
        </div>
        {containers.length === 0 ? (
          <p className="text-slate-500 text-sm">
            No container data yet — the Docker adapter is wired in a later phase (via the
            docker-socket-proxy).
          </p>
        ) : (
          <div className="grid gap-3 grid-cols-[repeat(auto-fit,minmax(260px,1fr))]">
            {containers.map((c) => (
              <ContainerCard key={c.id} c={c} />
            ))}
          </div>
        )}
      </section>

      <section>
        <h2 className="text-sm font-semibold text-slate-300 mb-2">Storage</h2>
        {disks.length === 0 ? (
          <p className="text-slate-500 text-sm">No disk data yet.</p>
        ) : (
          <div className="grid gap-3 grid-cols-[repeat(auto-fit,minmax(260px,1fr))]">
            {disks.map((d) => (
              <DiskGauge key={d.path} d={d} />
            ))}
          </div>
        )}
      </section>

      <section>
        <h2 className="text-sm font-semibold text-slate-300 mb-2">Notifications</h2>
        <TestNotifyButton />
      </section>
    </div>
  )
}

function TestNotifyButton() {
  const [state, setState] = useState<'idle' | 'sending' | 'ok' | 'err'>('idle')
  const [msg, setMsg] = useState('')

  const send = async () => {
    setState('sending')
    try {
      const r = await fetch('/api/notify/test', { method: 'POST' })
      if (r.ok) {
        setState('ok')
        setMsg('')
      } else {
        setState('err')
        setMsg(await r.text())
      }
    } catch (e) {
      setState('err')
      setMsg(String(e))
    }
    window.setTimeout(() => setState('idle'), 4000)
  }

  return (
    <div className="flex items-center gap-3">
      <button
        onClick={send}
        disabled={state === 'sending'}
        className="text-sm px-3 py-1.5 rounded-md bg-slate-700 text-slate-200 hover:bg-slate-600 disabled:opacity-50"
      >
        {state === 'sending' ? 'Sending…' : 'Send test notification'}
      </button>
      {state === 'ok' && <span className="text-emerald-300 text-sm">Sent ✓</span>}
      {state === 'err' && <span className="text-rose-300 text-sm">{msg || 'Failed'}</span>}
    </div>
  )
}

function ContainerCard({ c }: { c: Container }) {
  const { send } = useWs()
  const up = c.state === 'running'
  return (
    <div className="rounded-lg border border-slate-800 bg-slate-900/60 p-3 flex flex-col gap-2">
      <div className="flex items-center gap-2">
        <span className="font-medium text-sm">{c.name}</span>
        <span
          className={cn(
            'ml-auto text-xs px-2 py-0.5 rounded',
            up ? 'bg-emerald-500/20 text-emerald-300' : 'bg-rose-500/20 text-rose-300',
          )}
        >
          {c.state}
        </span>
      </div>
      <div className="text-xs text-slate-400">{c.status}</div>
      {up && (
        <div className="flex gap-3 text-xs text-slate-400">
          <span>CPU {c.cpuPct.toFixed(0)}%</span>
          <span>Mem {fmtBytes(c.memUsed)}</span>
          <span>Up {fmtDuration(c.uptimeSec)}</span>
        </div>
      )}
      <button
        onClick={() => {
          if (confirm(`Restart ${c.name}?`)) send({ type: 'container.restart', data: { id: c.id } })
        }}
        className="text-xs px-2 py-1 rounded bg-slate-700 text-slate-200 hover:bg-slate-600 self-start"
      >
        Restart
      </button>
    </div>
  )
}

function DiskGauge({ d }: { d: DiskUsage }) {
  const danger = d.usedPct > 90
  return (
    <div className="rounded-lg border border-slate-800 bg-slate-900/60 p-3 flex flex-col gap-2">
      <div className="flex justify-between text-sm">
        <span className="font-mono text-slate-300">{d.path}</span>
        <span className="text-slate-400">{d.usedPct.toFixed(0)}%</span>
      </div>
      <div className="h-2 rounded-full bg-slate-800 overflow-hidden">
        <div
          className={cn('h-full', danger ? 'bg-rose-500' : 'bg-sky-500')}
          style={{ width: `${Math.min(100, d.usedPct)}%` }}
        />
      </div>
      <div className="text-xs text-slate-500">
        {fmtBytes(d.used)} used · {fmtBytes(d.free)} free
      </div>
    </div>
  )
}
