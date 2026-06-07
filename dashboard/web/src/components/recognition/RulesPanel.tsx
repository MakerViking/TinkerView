import { useEffect, useState } from 'react'
import { inputCls } from '@/components/devices/forms'
import { useLive } from '@/store/live'
import type { NotifyRule } from '@/types/ws'
import { cn } from '@/lib/utils'

const OBJECTS = ['person', 'car', 'truck', 'bicycle', 'motorcycle', 'cat', 'dog', 'bird']
const ACTIONS: { value: NotifyRule['action']; label: string }[] = [
  { value: 'push', label: 'Push' },
  { value: 'silent', label: 'Silent' },
  { value: 'ignore', label: 'Ignore' },
]
type MatchMode = 'any' | 'known' | 'unknown' | 'name'

function matchMode(r: NotifyRule): MatchMode {
  const m = (r.match ?? '').trim().toLowerCase()
  if (m === '' || m === 'any') return 'any'
  if (m === 'known' || m === 'unknown') return m
  return 'name'
}

export function RulesPanel() {
  const cameras = Object.keys(useLive((s) => s.cameras)).sort()
  const [rules, setRules] = useState<NotifyRule[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [status, setStatus] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    fetch('/api/notify-rules')
      .then((r) => (r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`))))
      .then((rs: NotifyRule[]) => !cancelled && setRules(rs ?? []))
      .catch((e) => !cancelled && setError(e instanceof Error ? e.message : String(e)))
    return () => {
      cancelled = true
    }
  }, [])

  const patch = (i: number, p: Partial<NotifyRule>) =>
    setRules((rs) => rs!.map((r, idx) => (idx === i ? { ...r, ...p } : r)))
  const add = () =>
    setRules((rs) => [...(rs ?? []), { name: '', action: 'push', priority: 3 }])
  const remove = (i: number) => setRules((rs) => rs!.filter((_, idx) => idx !== i))
  const move = (i: number, dir: -1 | 1) =>
    setRules((rs) => {
      const next = [...rs!]
      const j = i + dir
      if (j < 0 || j >= next.length) return next
      ;[next[i], next[j]] = [next[j], next[i]]
      return next
    })

  const save = async () => {
    setError(null)
    setStatus(null)
    try {
      const r = await fetch('/api/notify-rules', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(rules ?? []),
      })
      if (!r.ok) throw new Error((await r.text()) || `HTTP ${r.status}`)
      setStatus('Rules saved.')
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }

  if (!rules) {
    return error ? (
      <div className="text-sm text-rose-300">{error}</div>
    ) : (
      <div className="p-8 text-center text-slate-400">Loading…</div>
    )
  }

  return (
    <div className="flex max-w-3xl flex-col gap-3">
      <p className="text-xs text-slate-500">
        When a detection fires, the <span className="text-slate-300">first matching rule wins</span>. Empty
        selectors mean "any". Use it to make recognized people/plates silent and unknown visitors loud. Object
        & name matching needs face/plate recognition enrolled under the other tabs.
      </p>

      <div className="flex flex-col gap-2">
        {rules.map((r, i) => (
          <RuleCard
            key={i}
            r={r}
            cameras={cameras}
            first={i === 0}
            last={i === rules.length - 1}
            onPatch={(p) => patch(i, p)}
            onRemove={() => remove(i)}
            onMove={(d) => move(i, d)}
          />
        ))}
        {rules.length === 0 && (
          <div className="rounded-lg border border-dashed border-slate-800 p-6 text-center text-sm text-slate-500">
            No rules — every detection sends a normal push. Add a rule to prioritize or silence.
          </div>
        )}
      </div>

      <div className="flex items-center gap-2">
        <button onClick={add} className="text-sm text-sky-400 hover:text-sky-300">
          + Add rule
        </button>
        <button
          onClick={save}
          className="ml-auto rounded-md bg-sky-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-sky-500"
        >
          Save rules
        </button>
      </div>
      {error && <div className="text-sm text-rose-300">{error}</div>}
      {status && <div className="text-sm text-emerald-300">{status}</div>}
    </div>
  )
}

function RuleCard({
  r,
  cameras,
  first,
  last,
  onPatch,
  onRemove,
  onMove,
}: {
  r: NotifyRule
  cameras: string[]
  first: boolean
  last: boolean
  onPatch: (p: Partial<NotifyRule>) => void
  onRemove: () => void
  onMove: (dir: -1 | 1) => void
}) {
  const mode = matchMode(r)
  return (
    <div className="rounded-lg border border-slate-800 bg-slate-900/60 p-3">
      <div className="flex items-center gap-2">
        <input
          className={inputCls + ' mt-0 flex-1 font-medium'}
          placeholder="Rule name (e.g. Unknown person at night)"
          value={r.name ?? ''}
          onChange={(e) => onPatch({ name: e.target.value })}
        />
        <div className="flex shrink-0 items-center gap-0.5">
          <IconBtn disabled={first} onClick={() => onMove(-1)} title="Move up">
            ↑
          </IconBtn>
          <IconBtn disabled={last} onClick={() => onMove(1)} title="Move down">
            ↓
          </IconBtn>
          <IconBtn onClick={onRemove} title="Remove rule" tone="danger">
            ✕
          </IconBtn>
        </div>
      </div>

      <div className="mt-2 grid grid-cols-2 gap-2 sm:grid-cols-4">
        <Labeled label="Camera">
          <select
            className={inputCls + ' mt-0'}
            value={r.camera ?? ''}
            onChange={(e) => onPatch({ camera: e.target.value })}
          >
            <option value="">Any</option>
            {cameras.map((c) => (
              <option key={c} value={c}>
                {c}
              </option>
            ))}
          </select>
        </Labeled>

        <Labeled label="Object">
          <input
            className={inputCls + ' mt-0'}
            list="rule-objects"
            placeholder="Any"
            value={r.label ?? ''}
            onChange={(e) => onPatch({ label: e.target.value })}
          />
          <datalist id="rule-objects">
            {OBJECTS.map((o) => (
              <option key={o} value={o} />
            ))}
          </datalist>
        </Labeled>

        <Labeled label="Recognized">
          <select
            className={inputCls + ' mt-0'}
            value={mode}
            onChange={(e) => {
              const v = e.target.value as MatchMode
              // any → clear; known/unknown → that keyword; name → keep any existing
              // free-text name, else blank for the user to fill in.
              if (v === 'any') onPatch({ match: '' })
              else if (v === 'name') onPatch({ match: mode === 'name' ? r.match : '' })
              else onPatch({ match: v })
            }}
          >
            <option value="any">Any</option>
            <option value="known">Known (recognized)</option>
            <option value="unknown">Unknown</option>
            <option value="name">Specific name…</option>
          </select>
          {mode === 'name' && (
            <input
              className={inputCls}
              placeholder="exact name / plate owner"
              value={r.match ?? ''}
              onChange={(e) => onPatch({ match: e.target.value })}
            />
          )}
        </Labeled>

        <Labeled label="Zone">
          <input
            className={inputCls + ' mt-0'}
            placeholder="Any"
            value={r.zone ?? ''}
            onChange={(e) => onPatch({ zone: e.target.value })}
          />
        </Labeled>
      </div>

      <div className="mt-2 grid grid-cols-2 gap-2 sm:grid-cols-4">
        <Labeled label="From (local)">
          <input
            type="time"
            className={inputCls + ' mt-0'}
            value={r.from ?? ''}
            onChange={(e) => onPatch({ from: e.target.value })}
          />
        </Labeled>
        <Labeled label="To (local)">
          <input
            type="time"
            className={inputCls + ' mt-0'}
            value={r.to ?? ''}
            onChange={(e) => onPatch({ to: e.target.value })}
          />
        </Labeled>
        <Labeled label="Action">
          <select
            className={inputCls + ' mt-0'}
            value={r.action}
            onChange={(e) => onPatch({ action: e.target.value as NotifyRule['action'] })}
          >
            {ACTIONS.map((a) => (
              <option key={a.value} value={a.value}>
                {a.label}
              </option>
            ))}
          </select>
        </Labeled>
        <Labeled label="Priority">
          <select
            className={inputCls + ' mt-0 disabled:opacity-40'}
            disabled={r.action !== 'push'}
            value={r.priority ?? 3}
            onChange={(e) => onPatch({ priority: Number(e.target.value) })}
          >
            {[1, 2, 3, 4, 5].map((p) => (
              <option key={p} value={p}>
                {p}
              </option>
            ))}
          </select>
        </Labeled>
      </div>
    </div>
  )
}

function Labeled({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="text-[11px] text-slate-500">{label}</span>
      {children}
    </label>
  )
}

function IconBtn({
  children,
  onClick,
  disabled,
  title,
  tone,
}: {
  children: React.ReactNode
  onClick: () => void
  disabled?: boolean
  title: string
  tone?: 'danger'
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      title={title}
      className={cn(
        'h-6 w-6 rounded text-sm leading-none disabled:opacity-30',
        tone === 'danger'
          ? 'text-slate-500 hover:bg-rose-950 hover:text-rose-300'
          : 'text-slate-400 hover:bg-slate-800 hover:text-slate-200',
      )}
    >
      {children}
    </button>
  )
}
