import { useState } from 'react'
import { useLive } from '@/store/live'
import { useWs } from '@/ws/WebSocketProvider'
import { useConfirm } from '@/components/ConfirmDialog'
import { Badge, Button, Card } from '@/components/ui'
import { IconBox, IconDisk } from '@/components/icons'
import type { Container, DiskUsage } from '@/types/ws'
import { cn, fmtBytes, fmtDuration } from '@/lib/utils'

export function HealthPanel() {
  const containers = useLive((s) => s.containers)
  const disks = useLive((s) => s.disks)
  const frigateOnline = useLive((s) => s.frigateOnline)

  return (
    <div className="flex flex-col gap-7">
      <section>
        <SectionTitle icon={<IconBox className="text-[14px]" />} title="Services" />
        <div className="mb-3 flex items-center gap-2 text-sm">
          <Badge tone={frigateOnline ? 'ok' : 'danger'} dot pulse={frigateOnline}>
            Frigate {frigateOnline ? 'online' : 'offline'}
          </Badge>
        </div>
        {containers.length === 0 ? (
          <p className="text-sm text-ink-500">
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
        <SectionTitle icon={<IconDisk className="text-[14px]" />} title="Storage" />
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
        <SectionTitle title="Notifications" />
        <TestNotifyButton />
      </section>
    </div>
  )
}

function SectionTitle({ icon, title }: { icon?: React.ReactNode; title: string }) {
  return (
    <h2 className="mb-3 flex items-center gap-1.5 font-display text-xs font-semibold uppercase tracking-[0.14em] text-ink-400">
      {icon}
      {title}
    </h2>
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
      {state === 'ok' && <span className="text-sm text-emerald-300">Sent ✓</span>}
      {state === 'err' && <span className="text-sm text-rose-300">{msg || 'Failed'}</span>}
    </div>
  )
}

function ContainerCard({ c }: { c: Container }) {
  const { send } = useWs()
  const { ask, confirmNode } = useConfirm()
  const up = c.state === 'running'
  return (
    <Card className="flex flex-col gap-2 p-3.5">
      <div className="flex items-center gap-2">
        <span className="font-display text-sm font-semibold text-ink-50">{c.name}</span>
        <span className="ml-auto" />
        <Badge tone={up ? 'ok' : 'danger'} dot pulse={up}>
          {c.state}
        </Badge>
      </div>
      <div className="text-xs text-ink-400">{c.status}</div>
      {up && (
        <div className="flex gap-3 font-mono text-[11px] text-ink-400">
          <span>CPU {c.cpuPct.toFixed(0)}%</span>
          <span>Mem {fmtBytes(c.memUsed)}</span>
          <span>Up {fmtDuration(c.uptimeSec)}</span>
        </div>
      )}
      <Button
        size="xs"
        variant="outline"
        className="self-start"
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
    </Card>
  )
}

function DiskGauge({ d }: { d: DiskUsage }) {
  const tone =
    d.usedPct > 90 ? 'bg-rec' : d.usedPct > 70 ? 'bg-amber-400' : 'bg-gradient-to-r from-brand-600 to-brand-400'
  return (
    <Card className="flex flex-col gap-2 p-3.5">
      <div className="flex justify-between text-sm">
        <span className="font-mono text-ink-200">{d.path}</span>
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
        className="h-2 overflow-hidden rounded-full bg-ink-800"
      >
        <div className={cn('h-full rounded-full', tone)} style={{ width: `${Math.min(100, d.usedPct)}%` }} />
      </div>
      <div className="font-mono text-[11px] text-ink-500">
        {fmtBytes(d.used)} used · {fmtBytes(d.free)} free
      </div>
    </Card>
  )
}
