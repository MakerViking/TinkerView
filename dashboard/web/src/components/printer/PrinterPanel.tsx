import { useState } from 'react'
import { useLive } from '@/store/live'
import { useWs } from '@/ws/WebSocketProvider'
import { VideoStream } from '@/components/VideoStream'
import { DeviceMenu, type MenuAction } from '@/components/devices/DeviceMenu'
import { EditPrinterModal } from '@/components/devices/EditPrinterModal'
import { useConfirm } from '@/components/ConfirmDialog'
import { Button, Card, Segmented } from '@/components/ui'
import { IconChevronDown, IconExternal, IconFan, IconFile, IconOctagonStop } from '@/components/icons'
import { PrinterControls } from './PrinterControls'
import { FilesPanel } from './FilesPanel'
import { HistoryPanel } from './HistoryPanel'
import type { Heater, PState } from '@/types/ws'
import { cn, fmtDuration } from '@/lib/utils'

type DetailTab = 'control' | 'files' | 'history'

const STATE_STYLE: Record<PState, { text: string; dot: string; pulse?: boolean }> = {
  printing: { text: 'text-brand-400', dot: 'bg-brand-400', pulse: true },
  paused: { text: 'text-brand-400', dot: 'bg-brand-400' },
  complete: { text: 'text-ink-300', dot: 'bg-emerald-400' },
  error: { text: 'text-rose-400', dot: 'bg-rec' },
  cancelled: { text: 'text-ink-400', dot: 'bg-ink-600' },
  idle: { text: 'text-ink-400', dot: 'bg-ink-600' },
  offline: { text: 'text-ink-500', dot: 'bg-ink-700' },
}

const humanize = (k: string) => {
  const t = k.replace(/_/g, ' ')
  return t.charAt(0).toUpperCase() + t.slice(1)
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

  const stateStyle = STATE_STYLE[p.state]

  return (
    <Card className={cn('flex flex-col', p.state === 'offline' && 'opacity-75')}>
      <div className="flex items-center gap-2 p-4 pb-0">
        {p.uiUrl ? (
          <a
            href={p.uiUrl}
            target="_blank"
            rel="noreferrer"
            title={`Open ${p.name} web UI (${p.uiUrl})`}
            className="inline-flex items-center gap-1.5 text-[15px] font-semibold text-ink-50 transition-colors hover:text-brand-300"
          >
            {p.name}
            <IconExternal className="text-[12px] opacity-50" />
          </a>
        ) : (
          <span className="text-[15px] font-semibold text-ink-50">{p.name}</span>
        )}
        <span className="text-[12px] capitalize text-ink-500">{p.kind}</span>
        <span className="ml-auto" />
        <span className={cn('inline-flex items-center gap-1.5 text-[12px] capitalize', stateStyle.text)}>
          <span className={cn('h-1.5 w-1.5 rounded-full', stateStyle.dot, stateStyle.pulse && 'animate-blip')} />
          {p.state}
        </span>
        {menuActions.length > 0 && <DeviceMenu actions={menuActions} />}
      </div>

      {p.camera && (
        <div className="mx-4 mt-4 aspect-video overflow-hidden rounded-md bg-ink-1000">
          <VideoStream src={p.camera} className="h-full w-full object-contain" />
        </div>
      )}

      {p.filename && (
        <div className="mx-4 mt-4 flex min-w-0 items-center gap-1.5 text-[13px] text-ink-300">
          <IconFile className="shrink-0 text-[13px] text-ink-500" />
          <span className="truncate font-mono">{p.filename}</span>
        </div>
      )}

      <div className="mx-4 mt-3"><Progress pct={p.progressPct} eta={p.etaSec} layer={p.layer} total={p.totalLayers} active={p.state === 'printing'} /></div>

      <div className="mx-4 mt-4 grid grid-cols-2 gap-x-4 gap-y-2 sm:grid-cols-3">
        <TempChip label="Nozzle" h={p.nozzle} />
        <TempChip label="Bed" h={p.bed} />
        {p.chamber && <TempChip label="Chamber" h={p.chamber} />}
      </div>

      {(p.fans?.length || p.filamentSensor || (p.hostLoad && p.hostLoad.cpuPct > 0)) && (
        <div className="mx-4 mt-4 flex flex-wrap gap-x-4 gap-y-1 text-[12px] text-ink-400">
          {p.fans?.map((f) => (
            <span key={f.key} className={cn('inline-flex items-center gap-1', f.speed <= 0 && 'opacity-50')}>
              <IconFan className={cn('text-[12px]', f.speed > 0 && 'animate-spin [animation-duration:1.6s]')} />
              {humanize(f.key === 'part' ? 'fan' : f.key)}{' '}
              <span className="font-mono tabular-nums text-ink-200">{Math.round(f.speed * 100)}%</span>
            </span>
          ))}
          {p.filamentSensor && (
            <span className={p.filamentSensor.detected ? 'text-emerald-400' : 'text-brand-400'}>
              filament {p.filamentSensor.detected ? 'present' : 'out'}
            </span>
          )}
          {p.hostLoad && p.hostLoad.cpuPct > 0 && <span>host {p.hostLoad.cpuPct.toFixed(0)}%</span>}
        </div>
      )}

      {p.flags.controlBlocked ? (
        <div className="mx-4 mt-4 rounded-md border border-brand-400/25 bg-brand-400/10 px-3 py-2 text-[13px] leading-relaxed text-brand-300">
          {p.message || 'Control unavailable for this printer.'}
        </div>
      ) : (
        <div className="mx-4 mt-4 flex gap-2">
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
              variant="danger-soft"
              onClick={estop}
              title="Emergency stop — halts the printer and heaters immediately"
              className="border border-rec/40"
            >
              <IconOctagonStop className="text-[15px]" />
              E-Stop
            </Button>
          )}
        </div>
      )}

      {!p.caps && <div className="pb-4" />}
      {p.caps && (
        <div className="mt-4 border-t border-ink-850">
          <button
            onClick={() => setExpanded((e) => !e)}
            aria-expanded={expanded}
            className="flex w-full items-center justify-center gap-1.5 px-4 py-2 text-[13px] text-ink-400 transition-colors hover:text-ink-100"
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

      {editing && <EditPrinterModal id={id} onClose={() => setEditing(false)} />}
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
    <div className="flex flex-col gap-2">
      <div
        role="progressbar"
        aria-valuenow={Math.round(clamped)}
        aria-valuemin={0}
        aria-valuemax={100}
        className="h-1 overflow-hidden rounded-full bg-ink-850"
      >
        <div
          className={cn('h-full rounded-full transition-[width] duration-500', active ? 'bg-brand-400' : 'bg-ink-600')}
          style={{ width: `${clamped}%` }}
        />
      </div>
      <div className="flex items-center gap-3 font-mono text-[12px] tabular-nums text-ink-400">
        <span className={cn(active && 'text-ink-100')}>{pct.toFixed(0)}%</span>
        {total > 0 && (
          <span>
            layer {layer}/{total}
          </span>
        )}
        <span className="ml-auto">{eta > 0 ? `${fmtDuration(eta)} left` : ''}</span>
      </div>
    </div>
  )
}

function TempChip({ label, h }: { label: string; h: Heater }) {
  const heating = h.target > 0 && h.actual < h.target - 2
  return (
    <div className="min-w-0">
      <div className="flex items-center gap-1.5 text-[12px] text-ink-400">
        {label}
        {heating && <span className="h-1 w-1 animate-blip rounded-full bg-brand-400" title="heating" />}
      </div>
      <div className="font-mono text-lg tabular-nums leading-tight">
        <span className={heating ? 'text-brand-300' : 'text-ink-100'}>{h.actual.toFixed(0)}°</span>
        <span className="text-[13px] text-ink-500"> / {h.target.toFixed(0)}°</span>
      </div>
    </div>
  )
}
