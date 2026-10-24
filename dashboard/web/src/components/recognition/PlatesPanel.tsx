import { useEffect, useState } from 'react'
import { inputCls } from '@/components/devices/forms'
import { Button, Notice, Skeleton } from '@/components/ui'
import { IconPlus, IconX } from '@/components/icons'

// A known-plate row: an owner name and its plate strings (comma-separated in the
// input). Backed by Frigate's lpr.known_plates via the dashboard's /api/plates.
interface Row {
  name: string
  plates: string // comma/space separated
}

export function PlatesPanel() {
  const [rows, setRows] = useState<Row[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [status, setStatus] = useState<string | null>(null)
  const [unavailable, setUnavailable] = useState(false)

  useEffect(() => {
    let cancelled = false
    fetch('/api/plates')
      .then(async (r) => {
        if (r.status === 503) {
          setUnavailable(true)
          return null
        }
        if (!r.ok) throw new Error(`HTTP ${r.status}`)
        return (await r.json()) as { plates: Record<string, string[]> }
      })
      .then((d) => {
        if (cancelled || !d) return
        const next = Object.entries(d.plates ?? {})
          .map(([name, plates]) => ({ name, plates: plates.join(', ') }))
          .sort((a, b) => a.name.localeCompare(b.name))
        setRows(next.length ? next : [{ name: '', plates: '' }])
      })
      .catch((e) => !cancelled && setError(e instanceof Error ? e.message : String(e)))
    return () => {
      cancelled = true
    }
  }, [])

  const update = (i: number, patch: Partial<Row>) =>
    setRows((rs) => rs!.map((r, idx) => (idx === i ? { ...r, ...patch } : r)))
  const addRow = () => setRows((rs) => [...(rs ?? []), { name: '', plates: '' }])
  const removeRow = (i: number) => setRows((rs) => rs!.filter((_, idx) => idx !== i))

  const save = async () => {
    setError(null)
    setStatus(null)
    const plates: Record<string, string[]> = {}
    for (const r of rows ?? []) {
      const name = r.name.trim()
      const list = r.plates
        .split(/[,\n]/)
        .map((s) => s.trim())
        .filter(Boolean)
      if (!name && list.length === 0) continue // skip empty rows
      if (!name) {
        setError('Every row needs an owner name.')
        return
      }
      if (list.length === 0) {
        setError(`"${name}" has no plates.`)
        return
      }
      plates[name] = list
    }
    try {
      const r = await fetch('/api/plates', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ plates }),
      })
      if (r.status === 202) {
        setStatus('Saving — Frigate will restart (~25s). Watch the toast for the result.')
        return
      }
      if (!r.ok) throw new Error((await r.text()) || `HTTP ${r.status}`)
      setStatus('Saved.')
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }

  if (unavailable) {
    return (
      <Notice>Frigate config editing isn't available, so known plates can't be managed here.</Notice>
    )
  }
  if (!rows) {
    return (
      <div className="flex max-w-2xl flex-col gap-2" aria-label="Loading plates">
        <Skeleton className="h-10" />
        <Skeleton className="h-10" />
      </div>
    )
  }

  return (
    <div className="flex max-w-2xl flex-col gap-3">
      <p className="max-w-2xl text-sm leading-relaxed text-ink-400">
        Map a name to one or more license plates. Recognized plates show that name on detections and can be
        prioritized or silenced under <span className="text-ink-200">Notification rules</span> (e.g. your own
        car = silent). Saving restarts Frigate (~25s). Plates may be exact text or a regex.
      </p>
      <div className="flex flex-col gap-2">
        {rows.map((r, i) => (
          <div key={i} className="flex items-start gap-2">
            <input
              className={inputCls + ' !mt-0 w-40 shrink-0'}
              placeholder="Owner (e.g. Thomas)"
              aria-label="Owner name"
              value={r.name}
              onChange={(e) => update(i, { name: e.target.value })}
            />
            <input
              className={inputCls + ' !mt-0 flex-1 font-mono'}
              placeholder="Plates, comma-separated (e.g. B-AB1234, B-CD5678)"
              aria-label="Plates"
              value={r.plates}
              onChange={(e) => update(i, { plates: e.target.value })}
            />
            <button
              onClick={() => removeRow(i)}
              className="mt-2 grid h-6 w-6 place-items-center rounded-md text-ink-500 transition-colors hover:bg-rec/10 hover:text-rose-300"
              title="Remove row"
              aria-label="Remove row"
            >
              <IconX className="text-[13px]" />
            </button>
          </div>
        ))}
      </div>
      <div className="flex items-center gap-2">
        <Button variant="ghost" size="sm" onClick={addRow} className="text-brand-300 hover:text-brand-200">
          <IconPlus className="text-[13px]" />
          Add plate
        </Button>
        <Button variant="primary" onClick={save} className="ml-auto">
          Save plates
        </Button>
      </div>
      {error && <div className="text-sm text-rose-300">{error}</div>}
      {status && <div className="text-sm text-ink-300">{status}</div>}
    </div>
  )
}
