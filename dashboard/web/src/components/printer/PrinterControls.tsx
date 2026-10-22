import { type ReactNode, useEffect, useRef, useState } from 'react'
import { useWs } from '@/ws/WebSocketProvider'
import { useConfirm } from '@/components/ConfirmDialog'
import { Button } from '@/components/ui'
import { IconHome } from '@/components/icons'
import type { FanCap, Heater, PrinterAction, PrinterState } from '@/types/ws'
import { cn } from '@/lib/utils'

// PrinterControls is the expandable "Control" tab: every section is gated on the
// printer's advertised capabilities, so the U1 (no chamber/aux fans/sensor) shows
// fewer controls than the Qidi.
export function PrinterControls({ p }: { p: PrinterState }) {
  const { send } = useWs()
  const ctl = (action: PrinterAction) => send({ type: 'printer.control', data: { id: p.id, action } })
  const setTemp = (heater: string, target: number) =>
    send({ type: 'printer.temp', data: { id: p.id, heater, target } })
  const caps = p.caps
  if (!caps) return <div className="text-xs text-ink-500">No controls available for this printer.</div>
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
    <div className="rounded-xl border border-ink-800 bg-ink-950/40 p-3">
      <div className="mb-2.5 flex items-center">
        <span className="font-display text-[10px] font-semibold uppercase tracking-[0.12em] text-ink-400">
          {title}
        </span>
        {right && <span className="ml-auto font-mono text-[11px] text-ink-400">{right}</span>}
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
      <div className="mb-3 flex flex-wrap gap-1.5">
        {presets.map((pr) => (
          <Button key={pr.name} size="xs" variant="outline" onClick={() => apply(pr)}>
            {pr.name}
          </Button>
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
    // Number('') is 0, which would switch the heater off — require real input.
    if (v.trim() === '') return
    const t = Number(v)
    if (!Number.isFinite(t) || t < 0) return
    onSet(t)
    setV('')
  }
  return (
    <div>
      <div className="text-[11px] text-ink-400">
        {label}{' '}
        <span className="font-mono text-ink-100">{h.actual.toFixed(0)}°</span>
        <span className="font-mono text-ink-500"> / {h.target.toFixed(0)}°</span>
      </div>
      <div className="mt-1 flex gap-1">
        <input
          type="number"
          value={v}
          onChange={(e) => setV(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && submit()}
          placeholder="°C"
          aria-label={`${label} target temperature`}
          className="w-14 rounded-md border border-ink-700 bg-ink-950/70 px-1.5 py-1 font-mono text-xs text-ink-100 placeholder:text-ink-500 focus:border-brand-500 focus:outline-none"
        />
        <Button size="xs" onClick={submit}>
          Set
        </Button>
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
  const vRef = useRef(value)
  const dragging = useRef(false)
  useEffect(() => {
    if (!dragging.current) {
      setV(value)
      vRef.current = value
    }
  }, [value])
  // commit ends a drag / keyboard adjustment and sends the value once. Pointer
  // cancel, lost capture, blur and key release all funnel here so `dragging` can
  // never stay stuck (e.g. a touch scroll takes over and no pointerup arrives).
  const commit = () => {
    if (!dragging.current) return
    dragging.current = false
    onCommit(vRef.current)
  }
  return (
    <div className="mb-2.5 last:mb-0">
      <div className="flex text-[11px] text-ink-400">
        <span>{label}</span>
        <span className="ml-auto font-mono text-ink-100">{v}%</span>
      </div>
      <input
        type="range"
        min={min}
        max={max}
        value={v}
        aria-label={label}
        onPointerDown={() => (dragging.current = true)}
        onChange={(e) => {
          const n = Number(e.target.value)
          vRef.current = n
          setV(n)
          dragging.current = true // keyboard changes have no pointerdown
        }}
        onPointerUp={commit}
        onPointerCancel={commit}
        onLostPointerCapture={commit}
        onKeyUp={commit}
        onBlur={commit}
        className="w-full"
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
        <Button size="xs" onClick={() => ctl({ op: 'babystep', dist: -step })}>
          − {step} mm
        </Button>
        <Button size="xs" onClick={() => ctl({ op: 'babystep', dist: step })}>
          + {step} mm
        </Button>
      </div>
    </Section>
  )
}

function MoveSection({ ctl, idle }: { ctl: (a: PrinterAction) => void; idle: boolean }) {
  const [step, setStep] = useState(10)
  const steps = [0.1, 1, 10, 50]
  return (
    <Section title="Move" right={idle ? undefined : 'idle only'}>
      <fieldset disabled={!idle} className={cn('flex flex-col gap-2.5', !idle && 'opacity-40')}>
        <div className="flex flex-wrap gap-1.5">
          <Button size="xs" onClick={() => ctl({ op: 'home', axes: '' })}>
            <IconHome className="text-[12px]" />
            Home all
          </Button>
          <Button size="xs" onClick={() => ctl({ op: 'home', axes: 'x' })}>
            Home X
          </Button>
          <Button size="xs" onClick={() => ctl({ op: 'home', axes: 'y' })}>
            Home Y
          </Button>
          <Button size="xs" onClick={() => ctl({ op: 'home', axes: 'z' })}>
            Home Z
          </Button>
        </div>
        <div className="flex items-center gap-1 text-[11px] text-ink-400">
          step
          {steps.map((s) => (
            <button
              key={s}
              onClick={() => setStep(s)}
              aria-pressed={step === s}
              className={cn(
                'rounded-md px-2 py-1 font-mono transition-colors',
                step === s
                  ? 'bg-brand-500/20 text-brand-200 ring-1 ring-inset ring-brand-400/30'
                  : 'bg-ink-800 text-ink-300 hover:bg-ink-700',
              )}
            >
              {s}
            </button>
          ))}
          mm
        </div>
        <div className="flex flex-wrap gap-3">
          <Jog axis="X" ctl={ctl} step={step} />
          <Jog axis="Y" ctl={ctl} step={step} />
          <Jog axis="Z" ctl={ctl} step={step} />
        </div>
        <div className="flex items-center gap-1.5 text-[11px] text-ink-400">
          extrude
          {[-10, -1, 1, 10].map((d) => (
            <Button key={d} size="xs" onClick={() => ctl({ op: 'extrude', dist: d })} className="font-mono">
              {d > 0 ? `+${d}` : d}
            </Button>
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
      <span className="w-3 font-mono text-[11px] text-ink-400">{axis}</span>
      <Button
        size="xs"
        aria-label={`Move ${axis} −${step} mm`}
        onClick={() => ctl({ op: 'move', axis: axis.toLowerCase(), dist: -step })}
        className="min-w-8 font-mono"
      >
        −
      </Button>
      <Button
        size="xs"
        aria-label={`Move ${axis} +${step} mm`}
        onClick={() => ctl({ op: 'move', axis: axis.toLowerCase(), dist: step })}
        className="min-w-8 font-mono"
      >
        +
      </Button>
    </div>
  )
}

function MacroSection({ macros, ctl }: { macros: string[]; ctl: (a: PrinterAction) => void }) {
  return (
    <Section title="Macros">
      <div className="flex flex-wrap gap-1.5">
        {macros.map((m) => (
          <Button key={m} size="xs" variant="outline" onClick={() => ctl({ op: 'macro', name: m })} className="font-mono">
            {m}
          </Button>
        ))}
      </div>
    </Section>
  )
}

function RestartSection({ ctl, name }: { ctl: (a: PrinterAction) => void; name: string }) {
  const { ask, confirmNode } = useConfirm()
  return (
    <Section title="Maintenance">
      <Button
        size="xs"
        onClick={() =>
          ask({
            title: `Firmware restart ${name}`,
            message: `Restart Klipper on ${name}?`,
            confirmLabel: 'Restart firmware',
            danger: true,
            onConfirm: () => ctl({ op: 'firmware_restart' }),
          })
        }
      >
        Firmware restart
      </Button>
      {confirmNode}
    </Section>
  )
}
