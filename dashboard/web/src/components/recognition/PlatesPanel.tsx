import { useEffect, useState } from 'react'
import { inputCls } from '@/components/devices/forms'

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
      <div className="rounded-lg border border-slate-800 bg-slate-900/60 p-3 text-sm text-slate-300">
        Frigate config editing isn't available, so known plates can't be managed here.
      </div>
    )
  }
  if (!rows) return <div className="p-8 text-center text-slate-400">Loading…</div>

  return (
    <div className="flex max-w-2xl flex-col gap-3">
      <p className="text-xs text-slate-500">
        Map a name to one or more license plates. Recognized plates show that name on detections and can be
        prioritized or silenced under <span className="text-slate-300">Notification rules</span> (e.g. your own
        car = silent). Saving restarts Frigate (~25s). Plates may be exact text or a regex.
      </p>
      <div className="flex flex-col gap-2">
        {rows.map((r, i) => (
          <div key={i} className="flex items-start gap-2">
            <input
              className={inputCls + ' w-40 shrink-0'}
              placeholder="Owner (e.g. Thomas)"
              value={r.name}
              onChange={(e) => update(i, { name: e.target.value })}
            />
            <input
              className={inputCls + ' flex-1'}
              placeholder="Plates, comma-separated (e.g. B-AB1234, B-CD5678)"
              value={r.plates}
              onChange={(e) => update(i, { plates: e.target.value })}
            />
            <button
              onClick={() => removeRow(i)}
              className="mt-1.5 px-1.5 text-slate-500 hover:text-rose-400"
              title="Remove row"
            >
              ✕
            </button>
          </div>
        ))}
      </div>
      <div className="flex items-center gap-2">
        <button onClick={addRow} className="text-sm text-sky-400 hover:text-sky-300">
          + Add plate
        </button>
        <button
          onClick={save}
          className="ml-auto rounded-md bg-sky-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-sky-500"
        >
          Save plates
        </button>
      </div>
      {error && <div className="text-sm text-rose-300">{error}</div>}
      {status && <div className="text-sm text-emerald-300">{status}</div>}
    </div>
  )
}
