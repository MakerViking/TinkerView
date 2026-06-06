import { useState, type ReactNode } from 'react'
import { useLive } from '@/store/live'
import { useWs } from '@/ws/WebSocketProvider'
import { VideoStream } from '@/components/VideoStream'
import { DeviceMenu, type MenuAction } from '@/components/devices/DeviceMenu'
import { AddPrinterModal } from '@/components/devices/AddPrinterModal'
import { PrinterControls } from './PrinterControls'
import { FilesPanel } from './FilesPanel'
import { HistoryPanel } from './HistoryPanel'
import type { Heater, PState } from '@/types/ws'
import { cn, fmtDuration } from '@/lib/utils'

type DetailTab = 'control' | 'files' | 'history'

const STATE_CLS: Record<PState, string> = {
  printing: 'bg-sky-500/20 text-sky-300',
  paused: 'bg-amber-500/20 text-amber-300',
  complete: 'bg-emerald-500/20 text-emerald-300',
  error: 'bg-rose-500/20 text-rose-300',
  cancelled: 'bg-slate-500/20 text-slate-300',
  idle: 'bg-slate-500/20 text-slate-300',
  offline: 'bg-slate-700/40 text-slate-500',
}

export function PrinterPanel({ id }: { id: string }) {
  const p = useLive((s) => s.printers[id])
  const { send } = useWs()
  const [editing, setEditing] = useState(false)
  const [expanded, setExpanded] = useState(false)
  const [tab, setTab] = useState<DetailTab>('control')
  if (!p) return null

  const cmd = (c: 'pause' | 'resume' | 'cancel') => send({ type: 'printer.cmd', data: { id, cmd: c } })
  const estop = () => {
    if (confirm(`EMERGENCY STOP ${p.name}? This halts the printer and heaters immediately.`))
      send({ type: 'printer.control', data: { id, action: { op: 'estop' } } })
  }

  const menuActions: MenuAction[] = p.managed
    ? [
        { label: 'Edit…', onClick: () => setEditing(true) },
        {
          label: 'Remove',
          danger: true,
          onClick: () => {
            if (confirm(`Remove printer ${p.name}?`)) send({ type: 'printer.remove', data: { id } })
          },
        },
      ]
    : []

  return (
    <div className="rounded-lg border border-slate-800 bg-slate-900/60 p-4 flex flex-col gap-3">
      <div className="flex items-center gap-2">
        {p.uiUrl ? (
          <a
            href={p.uiUrl}
            target="_blank"
            rel="noreferrer"
            title={`Open ${p.name} web UI (${p.uiUrl})`}
            className="font-medium hover:text-sky-300 inline-flex items-center gap-1"
          >
            {p.name}
            <span className="text-xs opacity-60">↗</span>
          </a>
        ) : (
          <span className="font-medium">{p.name}</span>
        )}
        <span className="text-[10px] uppercase px-1.5 py-0.5 rounded bg-slate-700/60 text-slate-300">
          {p.kind}
        </span>
        <span className={cn('ml-auto text-xs px-2 py-0.5 rounded', STATE_CLS[p.state])}>
          {p.state}
        </span>
        {menuActions.length > 0 && <DeviceMenu actions={menuActions} />}
      </div>

      {p.camera && (
        <div className="rounded-md overflow-hidden bg-black aspect-video">
          <VideoStream src={p.camera} className="w-full h-full object-contain" />
        </div>
      )}

      {p.filename && <div className="text-sm text-slate-400 truncate">{p.filename}</div>}

      <Progress pct={p.progressPct} eta={p.etaSec} layer={p.layer} total={p.totalLayers} />

      <div className="flex flex-wrap gap-3 text-sm">
        <Temp label="Nozzle" h={p.nozzle} />
        <Temp label="Bed" h={p.bed} />
        {p.chamber && <Temp label="Chamber" h={p.chamber} />}
      </div>

      {(p.fans?.length || p.filamentSensor || (p.hostLoad && p.hostLoad.cpuPct > 0)) && (
        <div className="flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-slate-500">
          {p.fans?.map((f) => (
            <span key={f.key}>
              {f.key === 'part' ? 'fan' : f.key} {Math.round(f.speed * 100)}%
            </span>
          ))}
          {p.filamentSensor && (
            <span className={p.filamentSensor.detected ? 'text-emerald-500' : 'text-amber-500'}>
              filament {p.filamentSensor.detected ? 'present' : 'out'}
            </span>
          )}
          {p.hostLoad && p.hostLoad.cpuPct > 0 && <span>host {p.hostLoad.cpuPct.toFixed(0)}%</span>}
        </div>
      )}

      {p.flags.controlBlocked ? (
        <div className="text-xs text-amber-300 bg-amber-500/10 rounded px-2 py-1.5">
          {p.message || 'Control unavailable for this printer.'}
        </div>
      ) : (
        <div className="flex gap-2">
          <Btn disabled={!p.flags.canPause} onClick={() => cmd('pause')}>
            Pause
          </Btn>
          <Btn disabled={!p.flags.canResume} onClick={() => cmd('resume')}>
            Resume
          </Btn>
          <Btn
            danger
            disabled={!p.flags.canCancel}
            onClick={() => {
              if (confirm(`Cancel the print on ${p.name}?`)) cmd('cancel')
            }}
          >
            Cancel
          </Btn>
          {p.caps?.emergencyStop && (
            <Btn danger onClick={estop}>
              E-Stop
            </Btn>
          )}
        </div>
      )}

      {p.caps && (
        <div className="border-t border-slate-800 pt-2">
          <button
            onClick={() => setExpanded((e) => !e)}
            className="text-xs text-slate-400 hover:text-slate-200"
          >
            {expanded ? 'Controls ▴' : 'Controls ▾'}
          </button>
          {expanded && (
            <div className="mt-2">
              <div className="mb-2 flex gap-1">
                <TabBtn active={tab === 'control'} onClick={() => setTab('control')}>
                  Control
                </TabBtn>
                {p.caps.fileBrowser && (
                  <TabBtn active={tab === 'files'} onClick={() => setTab('files')}>
                    Files
                  </TabBtn>
                )}
                {p.caps.history && (
                  <TabBtn active={tab === 'history'} onClick={() => setTab('history')}>
                    History
                  </TabBtn>
                )}
              </div>
              {tab === 'control' && <PrinterControls p={p} />}
              {tab === 'files' && p.caps.fileBrowser && <FilesPanel id={id} />}
              {tab === 'history' && p.caps.history && <HistoryPanel id={id} />}
            </div>
          )}
        </div>
      )}

      {editing && <AddPrinterModal existing={p} onClose={() => setEditing(false)} />}
    </div>
  )
}

function TabBtn({ active, onClick, children }: { active: boolean; onClick: () => void; children: ReactNode }) {
  return (
    <button
      onClick={onClick}
      className={cn(
        'rounded-md px-2.5 py-1 text-xs transition',
        active ? 'bg-sky-500/20 text-sky-300' : 'text-slate-400 hover:bg-slate-800',
      )}
    >
      {children}
    </button>
  )
}

function Progress({
  pct,
  eta,
  layer,
  total,
}: {
  pct: number
  eta: number
  layer: number
  total: number
}) {
  return (
    <div className="flex flex-col gap-1">
      <div className="h-2 rounded-full bg-slate-800 overflow-hidden">
        <div
          className="h-full bg-sky-500 transition-[width] duration-500"
          style={{ width: `${Math.max(0, Math.min(100, pct))}%` }}
        />
      </div>
      <div className="flex justify-between text-xs text-slate-400">
        <span>{pct.toFixed(0)}%</span>
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

function Temp({ label, h }: { label: string; h: Heater }) {
  return (
    <span className="text-slate-300">
      <span className="text-slate-500">{label} </span>
      {h.actual.toFixed(0)}°
      <span className="text-slate-500"> / {h.target.toFixed(0)}°</span>
    </span>
  )
}

function Btn({
  children,
  onClick,
  disabled,
  danger,
}: {
  children: ReactNode
  onClick: () => void
  disabled?: boolean
  danger?: boolean
}) {
  return (
    <button
      disabled={disabled}
      onClick={onClick}
      className={cn(
        'flex-1 text-sm px-3 py-1.5 rounded-md transition',
        disabled
          ? 'bg-slate-800 text-slate-600 cursor-not-allowed'
          : danger
            ? 'bg-rose-500/20 text-rose-300 hover:bg-rose-500/30'
            : 'bg-slate-700 text-slate-200 hover:bg-slate-600',
      )}
    >
      {children}
    </button>
  )
}
