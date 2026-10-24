import { useEffect, useState } from 'react'
import { inputCls } from '@/components/devices/forms'
import { useLive } from '@/store/live'
import { Button, Card, Skeleton } from '@/components/ui'
import { IconArrowDown, IconArrowUp, IconPlus, IconX } from '@/components/icons'
import type { NotifyRule } from '@/types/ws'
import { cn } from '@/lib/utils'

const OBJECTS = ['person', 'car', 'truck', 'bicycle', 'motorcycle', 'cat', 'dog', 'bird']
const ACTIONS: { value: NotifyRule['action']; label: string }[] = [
  { value: 'push', label: 'Push' },
  { value: 'silent', label: 'Silent' },
  { value: 'ignore', label: 'Ignore' },
]
type MatchMode = 'any' | 'known' | 'unknown' | 'name'

// A whitespace-only match (NAME_BLANK) is the "Specific name…" mode with no name
// typed yet: the backend trims it to "any", but here it keeps the name field visible
// (an empty string would read back as "any" and snap the select back).
const NAME_BLANK = ' '

function matchMode(r: NotifyRule): MatchMode {
  const raw = r.match ?? ''
  if (raw !== '' && raw.trim() === '') return 'name'
  const m = raw.trim().toLowerCase()
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
    // "Specific name…" with no name typed would be saved as "any" (notify on everything).
    const blank = (rules ?? []).findIndex((x) => matchMode(x) === 'name' && (x.match ?? '').trim() === '')
    if (blank >= 0) {
      setError(
        `Rule ${blank + 1} is set to "Specific name…" but no name is entered. Enter a name or choose another Recognized option.`,
      )
      return
    }
    try {
      const r = await fetch('/api/notify-rules', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify((rules ?? []).map((r) => ({ ...r, match: r.match?.trim() }))),
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
      <div className="flex max-w-3xl flex-col gap-2" aria-label="Loading rules">
        <Skeleton className="h-28" />
        <Skeleton className="h-28" />
      </div>
    )
  }

  return (
    <div className="flex max-w-3xl flex-col gap-3">
      <p className="max-w-2xl text-sm leading-relaxed text-ink-400">
        When a detection fires, the <span className="text-ink-200">first matching rule wins</span>. Empty
        selectors mean "any". Use it to make recognized people/plates silent and unknown visitors loud. Object
        & name matching needs face/plate recognition enrolled under the other tabs.
      </p>

      <div className="flex flex-col gap-2">
        {rules.map((r, i) => (
          <RuleCard
            key={i}
            r={r}
            index={i}
            cameras={cameras}
            first={i === 0}
            last={i === rules.length - 1}
            onPatch={(p) => patch(i, p)}
            onRemove={() => remove(i)}
            onMove={(d) => move(i, d)}
          />
        ))}
        {rules.length === 0 && (
          <div className="rounded-lg border border-dashed border-ink-800 p-6 text-center text-sm text-ink-500">
            No rules — every detection sends a normal push. Add a rule to prioritize or silence.
          </div>
        )}
      </div>

      <div className="flex items-center gap-2">
        <Button variant="ghost" size="sm" onClick={add} className="text-brand-400 hover:text-brand-300">
          <IconPlus className="text-[13px]" />
          Add rule
        </Button>
        <Button variant="primary" onClick={save} className="ml-auto">
          Save rules
        </Button>
      </div>
      {error && <div className="text-sm text-rose-300">{error}</div>}
      {status && <div className="text-sm text-ink-300">{status}</div>}
    </div>
  )
}

function RuleCard({
  r,
  index,
  cameras,
  first,
  last,
  onPatch,
  onRemove,
  onMove,
}: {
  r: NotifyRule
  index: number
  cameras: string[]
  first: boolean
  last: boolean
  onPatch: (p: Partial<NotifyRule>) => void
  onRemove: () => void
  onMove: (dir: -1 | 1) => void
}) {
  const mode = matchMode(r)
  return (
    <Card className="p-3.5">
      <div className="flex items-center gap-2">
        <span
          aria-hidden="true"
          className="grid h-6 w-6 shrink-0 place-items-center rounded-md bg-ink-800 font-mono text-[11px] text-ink-400"
          title={`Rule ${index + 1} — evaluated in order`}
        >
          {index + 1}
        </span>
        <input
          className={inputCls + ' !mt-0 flex-1 font-medium'}
          placeholder="Rule name (e.g. Unknown person at night)"
          aria-label="Rule name"
          value={r.name ?? ''}
          onChange={(e) => onPatch({ name: e.target.value })}
        />
        <div className="flex shrink-0 items-center gap-0.5">
          <IconBtn disabled={first} onClick={() => onMove(-1)} title="Move up">
            <IconArrowUp className="text-[13px]" />
          </IconBtn>
          <IconBtn disabled={last} onClick={() => onMove(1)} title="Move down">
            <IconArrowDown className="text-[13px]" />
          </IconBtn>
          <IconBtn onClick={onRemove} title="Remove rule" tone="danger">
            <IconX className="text-[13px]" />
          </IconBtn>
        </div>
      </div>

      <div className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
        <Labeled label="Camera">
          <select
            className={inputCls + ' !mt-0'}
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
            className={inputCls + ' !mt-0'}
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
            className={inputCls + ' !mt-0'}
            value={mode}
            onChange={(e) => {
              const v = e.target.value as MatchMode
              // any → clear; known/unknown → that keyword; name → keep any existing
              // free-text name, else a blank placeholder for the user to fill in.
              if (v === 'any') onPatch({ match: '' })
              else if (v === 'name') onPatch({ match: mode === 'name' ? r.match : NAME_BLANK })
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
              aria-label="Recognized name"
              value={(r.match ?? '').trim() === '' ? '' : r.match}
              onChange={(e) => onPatch({ match: e.target.value === '' ? NAME_BLANK : e.target.value })}
            />
          )}
        </Labeled>

        <Labeled label="Zone">
          <input
            className={inputCls + ' !mt-0'}
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
            className={inputCls + ' !mt-0'}
            value={r.from ?? ''}
            onChange={(e) => onPatch({ from: e.target.value })}
          />
        </Labeled>
        <Labeled label="To (local)">
          <input
            type="time"
            className={inputCls + ' !mt-0'}
            value={r.to ?? ''}
            onChange={(e) => onPatch({ to: e.target.value })}
          />
        </Labeled>
        <Labeled label="Action">
          <select
            className={inputCls + ' !mt-0'}
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
            className={inputCls + ' !mt-0 disabled:opacity-40'}
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
    </Card>
  )
}

function Labeled({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1.5 block text-[13px] font-medium text-ink-200">
        {label}
      </span>
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
      aria-label={title}
      className={cn(
        'grid h-7 w-7 place-items-center rounded-md transition-colors disabled:opacity-30',
        tone === 'danger'
          ? 'text-ink-500 hover:bg-rec/10 hover:text-rose-300'
          : 'text-ink-400 hover:bg-ink-800 hover:text-ink-100',
      )}
    >
      {children}
    </button>
  )
}
