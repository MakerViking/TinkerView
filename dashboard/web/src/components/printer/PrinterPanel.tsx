import { useState } from 'react'
import { useLive } from '@/store/live'
import { useWs } from '@/ws/WebSocketProvider'
import { VideoStream } from '@/components/VideoStream'
import { DeviceMenu, type MenuAction } from '@/components/devices/DeviceMenu'
import { AddPrinterModal } from '@/components/devices/AddPrinterModal'
import { useConfirm } from '@/components/ConfirmDialog'
import { Badge, Button, Card, Segmented, type BadgeTone } from '@/components/ui'
import { IconChevronDown, IconExternal, IconFan, IconFile, IconOctagonStop } from '@/components/icons'
import { PrinterControls } from './PrinterControls'
import { FilesPanel } from './FilesPanel'
import { HistoryPanel } from './HistoryPanel'
import type { Heater, PState } from '@/types/ws'
import { cn, fmtDuration } from '@/lib/utils'

type DetailTab = 'control' | 'files' | 'history'

const STATE_BADGE: Record<PState, { tone: BadgeTone; pulse?: boolean }> = {
  printing: { tone: 'brand', pulse: true },
  paused: { tone: 'warn' },
  complete: { tone: 'ok' },
  error: { tone: 'danger' },
  cancelled: { tone: 'neutral' },
  idle: { tone: 'neutral' },
  offline: { tone: 'neutral' },
}

export function PrinterPanel({ id }: { id: string }) {
  const p = useLive((s) => s.printers[id])
  const { send } = useWs()
  const [editing, setEditing] = useState(false)
  const [expanded, setExpanded] = useState(false)
  const [tab, setTab] = useState<DetailTab>('control')
  const { ask, confirmNode } = useConfirm()
  if (!p) return null

  const cmd = (c: 'pause' | 'resume' | 'cancel') => send({ type: 'printer.cmd', data: { id, cmd: c } })
  const estop = () =>
    ask({
      title: `Emergency stop ${p.name}`,
      message: 'This halts the printer and all heaters immediately. The print (if any) is lost.',
      confirmLabel: 'E-Stop now',
      danger: true,
      onConfirm: () => send({ type: 'printer.control', data: { id, action: { op: 'estop' } } }),
    })

  const menuActions: MenuAction[] = p.managed
    ? [
        { label: 'Edit…', onClick: () => setEditing(true) },
        {
          label: 'Remove',
          danger: true,
          onClick: () =>
            ask({
              title: `Remove ${p.name}`,
              message: `Remove printer ${p.name} from the dashboard?`,
              confirmLabel: 'Remove',
              danger: true,
              onConfirm: () => send({ type: 'printer.remove', data: { id } }),
            }),
        },
      ]
    : []

  const stateBadge = STATE_BADGE[p.state]

  return (
    <Card className={cn('flex flex-col gap-3.5 p-4', p.state === 'offline' && 'opacity-75')}>
      <div className="flex items-center gap-2">
        {p.uiUrl ? (
          <a
            href={p.uiUrl}
            target="_blank"
            rel="noreferrer"
            title={`Open ${p.name} web UI (${p.uiUrl})`}
            className="inline-flex items-center gap-1.5 font-display text-[15px] font-semibold tracking-wide text-ink-50 transition-colors hover:text-brand-300"
          >
            {p.name}
            <IconExternal className="text-[12px] opacity-50" />
          </a>
        ) : (
          <span className="font-display text-[15px] font-semibold tracking-wide text-ink-50">{p.name}</span>
        )}
        <span className="rounded-md bg-ink-800/80 px-1.5 py-0.5 font-mono text-[10px] uppercase tracking-wide text-ink-400">
          {p.kind}
        </span>
        <span className="ml-auto" />
        <Badge tone={stateBadge.tone} dot pulse={stateBadge.pulse}>
          {p.state}
        </Badge>
        {menuActions.length > 0 && <DeviceMenu actions={menuActions} />}
      </div>

      {p.camera && (
        <div className="aspect-video overflow-hidden rounded-lg border border-ink-800 bg-black">
          <VideoStream src={p.camera} className="h-full w-full object-contain" />
        </div>
      )}

      {p.filename && (
        <div className="flex min-w-0 items-center gap-1.5 text-xs text-ink-300">
          <IconFile className="shrink-0 text-[13px] text-ink-500" />
          <span className="truncate font-mono">{p.filename}</span>
        </div>
      )}

      <Progress pct={p.progressPct} eta={p.etaSec} layer={p.layer} total={p.totalLayers} active={p.state === 'printing'} />

      <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
        <TempChip label="Nozzle" h={p.nozzle} />
        <TempChip label="Bed" h={p.bed} />
        {p.chamber && <TempChip label="Chamber" h={p.chamber} />}
      </div>

      {(p.fans?.length || p.filamentSensor || (p.hostLoad && p.hostLoad.cpuPct > 0)) && (
        <div className="flex flex-wrap gap-x-3 gap-y-1 font-mono text-[11px] text-ink-500">
          {p.fans?.map((f) => (
            <span key={f.key} className="inline-flex items-center gap-1">
              <IconFan className={cn('text-[12px]', f.speed > 0 && 'animate-spin [animation-duration:1.6s]')} />
              {f.key === 'part' ? 'fan' : f.key} {Math.round(f.speed * 100)}%
            </span>
          ))}
          {p.filamentSensor && (
            <span className={p.filamentSensor.detected ? 'text-emerald-400' : 'text-amber-400'}>
              filament {p.filamentSensor.detected ? 'present' : 'out'}
            </span>
          )}
          {p.hostLoad && p.hostLoad.cpuPct > 0 && <span>host {p.hostLoad.cpuPct.toFixed(0)}%</span>}
        </div>
      )}

      {p.flags.controlBlocked ? (
        <div className="rounded-lg border border-amber-500/25 bg-amber-950/30 px-3 py-2 text-xs leading-relaxed text-amber-200">
          {p.message || 'Control unavailable for this printer.'}
        </div>
      ) : (
        <div className="flex gap-2">
          <Button className="flex-1" disabled={!p.flags.canPause} onClick={() => cmd('pause')}>
            Pause
          </Button>
          <Button className="flex-1" disabled={!p.flags.canResume} onClick={() => cmd('resume')}>
            Resume
          </Button>
          <Button
            variant="danger-soft"
            className="flex-1"
            disabled={!p.flags.canCancel}
            onClick={() =>
              ask({
                title: `Cancel print on ${p.name}`,
                message: `Cancel the running print on ${p.name}? Progress is lost.`,
                confirmLabel: 'Cancel print',
                danger: true,
                onConfirm: () => cmd('cancel'),
              })
            }
          >
            Cancel
          </Button>
          {p.caps?.emergencyStop && (
            <Button
              variant="outline"
              onClick={estop}
              title="Emergency stop — halts the printer and heaters immediately"
              className="!border-rec/50 !text-rose-300 hover:!border-rec hover:!bg-rec/10"
            >
              <IconOctagonStop className="text-[15px]" />
              E-Stop
            </Button>
          )}
        </div>
      )}

      {p.caps && (
        <div className="-mx-4 -mb-4 border-t border-ink-800">
          <button
            onClick={() => setExpanded((e) => !e)}
            aria-expanded={expanded}
            className="flex w-full items-center justify-center gap-1.5 px-4 py-2 font-display text-xs font-medium uppercase tracking-[0.1em] text-ink-400 transition-colors hover:text-ink-100"
          >
            Controls
            <IconChevronDown className={cn('text-[13px] transition-transform', expanded && 'rotate-180')} />
          </button>
          {expanded && (
            <div className="px-4 pb-4 animate-fade">
              <div className="mb-3">
                <Segmented
                  value={tab}
                  onChange={setTab}
                  options={[
                    { value: 'control' as const, label: 'Control' },
                    ...(p.caps.fileBrowser ? [{ value: 'files' as const, label: 'Files' }] : []),
                    ...(p.caps.history ? [{ value: 'history' as const, label: 'History' }] : []),
                  ]}
                />
              </div>
              {tab === 'control' && <PrinterControls p={p} />}
              {tab === 'files' && p.caps.fileBrowser && <FilesPanel id={id} />}
              {tab === 'history' && p.caps.history && <HistoryPanel id={id} />}
            </div>
          )}
        </div>
      )}

      {editing && <AddPrinterModal existing={p} onClose={() => setEditing(false)} />}
      {confirmNode}
    </Card>
  )
}

function Progress({
  pct,
  eta,
  layer,
  total,
  active,
}: {
  pct: number
  eta: number
  layer: number
  total: number
  active: boolean
}) {
  const clamped = Math.max(0, Math.min(100, pct))
  return (
    <div className="flex flex-col gap-1.5">
      <div
        role="progressbar"
        aria-valuenow={Math.round(clamped)}
        aria-valuemin={0}
        aria-valuemax={100}
        className="h-2 overflow-hidden rounded-full bg-ink-800"
      >
        <div
          className={cn(
            'h-full rounded-full transition-[width] duration-500',
            active
              ? 'bg-gradient-to-r from-brand-600 to-brand-400 shadow-[0_0_10px_rgb(56_189_248/0.55)]'
              : 'bg-ink-600',
          )}
          style={{ width: `${clamped}%` }}
        />
      </div>
      <div className="flex justify-between font-mono text-[11px] text-ink-400">
        <span className={cn(active && 'text-brand-300')}>{pct.toFixed(0)}%</span>
        {total > 0 && (
          <span>
            layer {layer}/{total}
          </span>
        )}
        <span>{eta > 0 ? `${fmtDuration(eta)} left` : ''}</span>
      </div>
    </div>
  )
}

function TempChip({ label, h }: { label: string; h: Heater }) {
  const heating = h.target > 0 && h.actual < h.target - 2
  return (
    <div
      className={cn(
        'rounded-lg border px-2.5 py-1.5 transition-colors',
        heating ? 'border-amber-500/30 bg-amber-950/20' : 'border-ink-800 bg-ink-950/40',
      )}
    >
      <div className="flex items-center gap-1 font-display text-[9px] font-semibold uppercase tracking-[0.12em] text-ink-500">
        {label}
        {heating && <span className="h-1 w-1 animate-blip rounded-full bg-amber-400" title="heating" />}
      </div>
      <div className="font-mono text-sm tabular-nums">
        <span className={heating ? 'text-amber-200' : 'text-ink-100'}>{h.actual.toFixed(0)}°</span>
        <span className="text-ink-500"> / {h.target.toFixed(0)}°</span>
      </div>
    </div>
  )
}
