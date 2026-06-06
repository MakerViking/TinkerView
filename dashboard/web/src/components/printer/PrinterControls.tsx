import { type ReactNode, useEffect, useRef, useState } from 'react'
import { useWs } from '@/ws/WebSocketProvider'
import type { FanCap, Heater, PrinterAction, PrinterState } from '@/types/ws'
import { cn } from '@/lib/utils'

const btn =
  'rounded px-2 py-1 text-xs bg-slate-700/70 text-slate-200 hover:bg-slate-600 disabled:opacity-40 disabled:cursor-not-allowed'

// PrinterControls is the expandable "Control" tab: every section is gated on the
// printer's advertised capabilities, so the U1 (no chamber/aux fans/sensor) shows
// fewer controls than the Qidi.
export function PrinterControls({ p }: { p: PrinterState }) {
  const { send } = useWs()
  const ctl = (action: PrinterAction) => send({ type: 'printer.control', data: { id: p.id, action } })
  const setTemp = (heater: string, target: number) =>
    send({ type: 'printer.temp', data: { id: p.id, heater, target } })
  const caps = p.caps
  if (!caps) return <div className="text-xs text-slate-500">No controls available for this printer.</div>
  const idle = ['idle', 'complete', 'cancelled', 'error'].includes(p.state)

  return (
    <div className="flex flex-col gap-3">
      {caps.setTemps && <TempSection p={p} setTemp={setTemp} />}
      {caps.speedFlow && <SpeedFlowSection p={p} ctl={ctl} />}
      {!!caps.fans?.length && <FanSection p={p} ctl={ctl} />}
      {caps.babystep && <BabystepSection p={p} ctl={ctl} />}
      {caps.move && <MoveSection ctl={ctl} idle={idle} />}
      {!!caps.macros?.length && <MacroSection macros={caps.macros} ctl={ctl} />}
      {caps.firmwareRestart && <RestartSection ctl={ctl} name={p.name} />}
    </div>
  )
}

function Section({ title, right, children }: { title: string; right?: ReactNode; children: ReactNode }) {
  return (
    <div className="rounded-md border border-slate-800 p-3">
      <div className="mb-2 flex items-center">
        <span className="text-[11px] uppercase tracking-wide text-slate-400">{title}</span>
        {right && <span className="ml-auto text-[11px] text-slate-400">{right}</span>}
      </div>
      {children}
    </div>
  )
}

function TempSection({ p, setTemp }: { p: PrinterState; setTemp: (h: string, t: number) => void }) {
  const presets: { name: string; n: number; b: number; c?: number }[] = [
    { name: 'Cooldown', n: 0, b: 0, c: 0 },
    { name: 'PLA', n: 210, b: 60 },
    { name: 'PETG', n: 240, b: 80 },
    { name: 'ABS', n: 245, b: 100, c: 50 },
  ]
  const apply = (pr: (typeof presets)[number]) => {
    setTemp('nozzle', pr.n)
    setTemp('bed', pr.b)
    if (p.caps?.chamber && pr.c != null) setTemp('chamber', pr.c)
  }
  return (
    <Section title="Temperatures">
      <div className="mb-2 flex flex-wrap gap-1.5">
        {presets.map((pr) => (
          <button key={pr.name} onClick={() => apply(pr)} className={btn}>
            {pr.name}
          </button>
        ))}
      </div>
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
        <HeaterInput label="Nozzle" h={p.nozzle} onSet={(t) => setTemp('nozzle', t)} />
        <HeaterInput label="Bed" h={p.bed} onSet={(t) => setTemp('bed', t)} />
        {p.caps?.chamber && p.chamber && (
          <HeaterInput label="Chamber" h={p.chamber} onSet={(t) => setTemp('chamber', t)} />
        )}
      </div>
    </Section>
  )
}

function HeaterInput({ label, h, onSet }: { label: string; h: Heater; onSet: (t: number) => void }) {
  const [v, setV] = useState('')
  const submit = () => {
    const t = Number(v)
    if (!isNaN(t)) {
      onSet(t)
      setV('')
    }
  }
  return (
    <div>
      <div className="text-[11px] text-slate-400">
        {label}{' '}
        <span className="text-slate-200">{h.actual.toFixed(0)}°</span>
        <span className="text-slate-500"> / {h.target.toFixed(0)}°</span>
      </div>
      <div className="mt-1 flex gap-1">
        <input
          type="number"
          value={v}
          onChange={(e) => setV(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && submit()}
          placeholder="°C"
          className="w-14 rounded bg-slate-800 border border-slate-700 px-1.5 py-0.5 text-xs"
        />
        <button onClick={submit} className={btn}>
          Set
        </button>
      </div>
    </div>
  )
}

function PctSlider({
  label,
  value,
  min,
  max,
  onCommit,
}: {
  label: string
  value: number
  min: number
  max: number
  onCommit: (v: number) => void
}) {
  const [v, setV] = useState(value)
  const dragging = useRef(false)
  useEffect(() => {
    if (!dragging.current) setV(value)
  }, [value])
  return (
    <div className="mb-2 last:mb-0">
      <div className="flex text-[11px] text-slate-400">
        <span>{label}</span>
        <span className="ml-auto text-slate-200">{v}%</span>
      </div>
      <input
        type="range"
        min={min}
        max={max}
        value={v}
        onPointerDown={() => (dragging.current = true)}
        onChange={(e) => setV(Number(e.target.value))}
        onPointerUp={() => {
          dragging.current = false
          onCommit(v)
        }}
        className="w-full accent-sky-500"
      />
    </div>
  )
}

function SpeedFlowSection({ p, ctl }: { p: PrinterState; ctl: (a: PrinterAction) => void }) {
  return (
    <Section title="Speed & Flow">
      <PctSlider label="Speed" value={Math.round(p.motion?.speedFactor ?? 100)} min={10} max={300} onCommit={(v) => ctl({ op: 'speed', pct: v })} />
      <PctSlider label="Flow" value={Math.round(p.motion?.flowFactor ?? 100)} min={50} max={200} onCommit={(v) => ctl({ op: 'flow', pct: v })} />
    </Section>
  )
}

function fanLabel(fc: FanCap): string {
  if (fc.kind === 'part') return 'Part cooling'
  return fc.key
}

function FanSection({ p, ctl }: { p: PrinterState; ctl: (a: PrinterAction) => void }) {
  const fans = p.caps?.fans ?? []
  const live = (key: string) => p.fans?.find((f) => f.key === key)?.speed ?? 0
  return (
    <Section title="Fans">
      {fans.map((fc) => (
        <PctSlider
          key={fc.key}
          label={fanLabel(fc)}
          value={Math.round(live(fc.key) * 100)}
          min={0}
          max={100}
          onCommit={(v) => ctl({ op: 'fan', name: fc.key, pct: v })}
        />
      ))}
    </Section>
  )
}

function BabystepSection({ p, ctl }: { p: PrinterState; ctl: (a: PrinterAction) => void }) {
  const step = 0.05
  return (
    <Section title="Z offset (babystep)" right={`${(p.motion?.zOffset ?? 0).toFixed(3)} mm`}>
      <div className="flex gap-2">
        <button onClick={() => ctl({ op: 'babystep', dist: -step })} className={btn}>
          − {step} mm
        </button>
        <button onClick={() => ctl({ op: 'babystep', dist: step })} className={btn}>
          + {step} mm
        </button>
      </div>
    </Section>
  )
}

function MoveSection({ ctl, idle }: { ctl: (a: PrinterAction) => void; idle: boolean }) {
  const [step, setStep] = useState(10)
  const steps = [0.1, 1, 10, 50]
  return (
    <Section title="Move" right={idle ? undefined : 'idle only'}>
      <fieldset disabled={!idle} className={cn('flex flex-col gap-2', !idle && 'opacity-40')}>
        <div className="flex flex-wrap gap-1">
          <button onClick={() => ctl({ op: 'home', axes: '' })} className={btn}>
            ⌂ Home all
          </button>
          <button onClick={() => ctl({ op: 'home', axes: 'x' })} className={btn}>
            Home X
          </button>
          <button onClick={() => ctl({ op: 'home', axes: 'y' })} className={btn}>
            Home Y
          </button>
          <button onClick={() => ctl({ op: 'home', axes: 'z' })} className={btn}>
            Home Z
          </button>
        </div>
        <div className="flex items-center gap-1 text-[11px] text-slate-400">
          step
          {steps.map((s) => (
            <button
              key={s}
              onClick={() => setStep(s)}
              className={cn('rounded px-1.5 py-0.5', step === s ? 'bg-sky-500/40 text-sky-100' : 'bg-slate-700/70 text-slate-300')}
            >
              {s}
            </button>
          ))}
          mm
        </div>
        <div className="flex flex-wrap gap-2">
          <Jog axis="X" ctl={ctl} step={step} />
          <Jog axis="Y" ctl={ctl} step={step} />
          <Jog axis="Z" ctl={ctl} step={step} />
        </div>
        <div className="flex items-center gap-1 text-[11px] text-slate-400">
          extrude
          {[-10, -1, 1, 10].map((d) => (
            <button key={d} onClick={() => ctl({ op: 'extrude', dist: d })} className={btn}>
              {d > 0 ? `+${d}` : d}
            </button>
          ))}
          mm
        </div>
      </fieldset>
    </Section>
  )
}

function Jog({ axis, step, ctl }: { axis: string; step: number; ctl: (a: PrinterAction) => void }) {
  return (
    <div className="flex items-center gap-1">
      <span className="w-3 text-[11px] text-slate-400">{axis}</span>
      <button onClick={() => ctl({ op: 'move', axis: axis.toLowerCase(), dist: -step })} className={btn}>
        −
      </button>
      <button onClick={() => ctl({ op: 'move', axis: axis.toLowerCase(), dist: step })} className={btn}>
        +
      </button>
    </div>
  )
}

function MacroSection({ macros, ctl }: { macros: string[]; ctl: (a: PrinterAction) => void }) {
  return (
    <Section title="Macros">
      <div className="flex flex-wrap gap-1.5">
        {macros.map((m) => (
          <button key={m} onClick={() => ctl({ op: 'macro', name: m })} className={btn}>
            {m}
          </button>
        ))}
      </div>
    </Section>
  )
}

function RestartSection({ ctl, name }: { ctl: (a: PrinterAction) => void; name: string }) {
  return (
    <Section title="Maintenance">
      <button
        onClick={() => {
          if (confirm(`Firmware restart ${name}? This restarts Klipper.`)) ctl({ op: 'firmware_restart' })
        }}
        className={btn}
      >
        Firmware restart
      </button>
    </Section>
  )
}
