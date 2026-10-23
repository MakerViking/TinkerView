import { useMemo, useState } from 'react'
import { useLive } from '@/store/live'
import { useWs } from '@/ws/WebSocketProvider'
import { useConfirm } from '@/components/ConfirmDialog'
import { Button, Card, SectionHeader } from '@/components/ui'
import type { Container, DiskUsage } from '@/types/ws'
import { cn, fmtBytes, fmtDuration } from '@/lib/utils'

// name | status | cpu | mem | uptime | action. On narrow screens CPU/Mem/Uptime
// collapse into a line under the name so the name keeps its width.
const ROW_COLS =
  'grid items-center gap-x-4 grid-cols-[minmax(0,1fr)_auto_auto] ' +
  'sm:grid-cols-[minmax(0,1fr)_6.5rem_4rem_5.5rem_6rem_auto]'

export function HealthPanel() {
  const containers = useLive((s) => s.containers)
  const disks = useLive((s) => s.disks)
  const frigateOnline = useLive((s) => s.frigateOnline)

  // Problems (not running) first, then by name — render-time only.
  const sorted = useMemo(
    () =>
      [...containers].sort((a, b) => {
        const pa = a.state === 'running' ? 1 : 0
        const pb = b.state === 'running' ? 1 : 0
        return pa - pb || a.name.localeCompare(b.name)
      }),
    [containers],
  )
  const running = containers.filter((c) => c.state === 'running').length
  const stopped = containers.length - running

  return (
    <div className="flex max-w-5xl flex-col gap-6">
      <SectionHeader
        title="Server"
        meta={
          <>
            {containers.length > 0 && `${running} running · ${stopped} stopped · `}
            <span className={frigateOnline ? undefined : 'text-rose-300'}>
              Frigate {frigateOnline ? 'online' : 'offline'}
            </span>
          </>
        }
      />

      <section>
        <h2 className="mb-3 text-sm font-semibold text-ink-100">Services</h2>
        {containers.length === 0 ? (
          <p className="text-sm text-ink-500">
            No container data yet — the Docker adapter is wired in a later phase (via the
            docker-socket-proxy).
          </p>
        ) : (
          <Card className="overflow-hidden">
            <div className={cn(ROW_COLS, 'border-b border-ink-850 px-4 py-2 text-[12px] text-ink-500')}>
              <span>Name</span>
              <span>Status</span>
              <span className="hidden text-right sm:block">CPU</span>
              <span className="hidden text-right sm:block">Memory</span>
              <span className="hidden text-right sm:block">Uptime</span>
              <span className="w-[4.25rem]" aria-hidden />
            </div>
            <div className="divide-y divide-ink-850">
              {sorted.map((c) => (
                <ContainerRow key={c.id} c={c} />
              ))}
            </div>
          </Card>
        )}
      </section>

      <section>
        <h2 className="mb-3 text-sm font-semibold text-ink-100">Storage</h2>
        {disks.length === 0 ? (
          <p className="text-sm text-ink-500">No disk data yet.</p>
        ) : (
          <div className="grid gap-3 grid-cols-[repeat(auto-fit,minmax(260px,1fr))]">
            {disks.map((d) => (
              <DiskGauge key={d.path} d={d} />
            ))}
          </div>
        )}
      </section>

      <section>
        <h2 className="mb-3 text-sm font-semibold text-ink-100">Notifications</h2>
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
      <Button onClick={send} disabled={state === 'sending'}>
        {state === 'sending' ? 'Sending…' : 'Send test notification'}
      </Button>
      {state === 'ok' && <span className="text-sm text-ink-300">Sent ✓</span>}
      {state === 'err' && <span className="text-sm text-rose-300">{msg || 'Failed'}</span>}
    </div>
  )
}

function ContainerRow({ c }: { c: Container }) {
  const { send } = useWs()
  const { ask, confirmNode } = useConfirm()
  const up = c.state === 'running'
  const exited = c.state === 'exited' || c.state === 'dead'
  const dotCls = up ? 'bg-emerald-400' : exited ? 'bg-rec' : 'bg-brand-400'
  const textCls = up ? 'text-ink-300' : exited ? 'text-rose-300' : 'text-brand-300'
  const state = c.state.charAt(0).toUpperCase() + c.state.slice(1)
  return (
    <div className={cn(ROW_COLS, 'group px-4 py-2.5 hover:bg-ink-850/50')}>
      <div className="min-w-0">
        <div className="truncate text-sm font-medium text-ink-100">{c.name}</div>
        {up && (
          <div className="font-mono text-[12px] tabular-nums text-ink-500 sm:hidden">
            {c.cpuPct.toFixed(0)}% · {fmtBytes(c.memUsed)} · {fmtDuration(c.uptimeSec)}
          </div>
        )}
      </div>
      <span className={cn('flex items-center gap-2 text-[13px]', textCls)} title={c.status}>
        <span className={cn('h-1.5 w-1.5 shrink-0 rounded-full', dotCls)} />
        {state}
      </span>
      <span className="hidden text-right font-mono text-[13px] tabular-nums text-ink-300 sm:block">
        {up ? `${c.cpuPct.toFixed(0)}%` : '—'}
      </span>
      <span className="hidden text-right font-mono text-[13px] tabular-nums text-ink-300 sm:block">
        {up ? fmtBytes(c.memUsed) : '—'}
      </span>
      <span className="hidden text-right font-mono text-[13px] tabular-nums text-ink-300 sm:block">
        {up ? fmtDuration(c.uptimeSec) : '—'}
      </span>
      <Button
        size="xs"
        variant="ghost"
        className="opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 focus-visible:opacity-100 pointer-coarse:opacity-100"
        onClick={() =>
          ask({
            title: `Restart ${c.name}`,
            message: `Restart the ${c.name} container?`,
            confirmLabel: 'Restart',
            danger: true,
            onConfirm: () => send({ type: 'container.restart', data: { id: c.id } }),
          })
        }
      >
        Restart
      </Button>
      {confirmNode}
    </div>
  )
}

function DiskGauge({ d }: { d: DiskUsage }) {
  const tone =
    d.usedPct > 90 ? 'bg-rec' : d.usedPct > 70 ? 'bg-brand-500' : 'bg-brand-400'
  return (
    <Card className="flex flex-col gap-2 p-4">
      <div className="flex justify-between text-sm">
        <span className="font-mono text-sm text-ink-100">{d.path}</span>
        <span className={cn('font-mono', d.usedPct > 90 ? 'text-rose-300' : 'text-ink-400')}>
          {d.usedPct.toFixed(0)}%
        </span>
      </div>
      <div
        role="progressbar"
        aria-valuenow={Math.round(d.usedPct)}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-label={`${d.path} used`}
        className="h-1.5 overflow-hidden rounded-full bg-ink-800"
      >
        <div className={cn('h-full rounded-full', tone)} style={{ width: `${Math.min(100, d.usedPct)}%` }} />
      </div>
      <div className="text-[12px] text-ink-500">
        {fmtBytes(d.used)} used · {fmtBytes(d.free)} free
      </div>
    </Card>
  )
}
