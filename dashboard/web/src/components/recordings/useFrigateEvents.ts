import { useCallback, useEffect, useRef, useState } from 'react'

// A Frigate tracked-object event (subset of fields we use). Reached through the
// dashboard's /frigate/ reverse proxy — no dashboard backend code involved.
export interface FrigateEvent {
  id: string
  camera: string
  label: string
  sub_label?: string | null
  zones?: string[]
  start_time: number // epoch seconds
  end_time?: number | null
  has_clip: boolean
  has_snapshot: boolean
  data?: { top_score?: number; score?: number }
}

export interface EventFilters {
  camera?: string
  label?: string
  zone?: string
  after?: number // epoch seconds
  before?: number
  limit?: number
}

export function frigateEventsUrl(f: EventFilters): string {
  const p = new URLSearchParams()
  if (f.camera) p.set('camera', f.camera)
  if (f.label) p.set('label', f.label)
  if (f.zone) p.set('zone', f.zone)
  if (f.after) p.set('after', String(f.after))
  if (f.before) p.set('before', String(f.before))
  p.set('limit', String(f.limit ?? 100))
  return `/frigate/api/events?${p.toString()}`
}

// useFrigateEvents fetches the events list from Frigate via the proxy. The filters
// object is collapsed to a stable URL string so the effect only re-runs when a
// filter actually changes.
export function useFrigateEvents(f: EventFilters) {
  const [data, setData] = useState<FrigateEvent[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  // Starts true so the first paint is the skeleton, not the "No recordings" empty state.
  const [loading, setLoading] = useState(true)
  const url = frigateEventsUrl(f)
  // reqRef ids the latest request so a slower, older response can't overwrite a
  // newer filter's data; dataUrlRef is the URL `data` belongs to.
  const reqRef = useRef(0)
  const dataUrlRef = useRef(url)

  const load = useCallback(async () => {
    const req = ++reqRef.current
    if (dataUrlRef.current !== url) {
      dataUrlRef.current = url
      setData(null) // filters changed: drop the previous filter's results
    }
    setLoading(true)
    setError(null)
    try {
      const r = await fetch(url)
      if (!r.ok) throw new Error(`HTTP ${r.status}`)
      const d = (await r.json()) as FrigateEvent[]
      if (req === reqRef.current) setData(d)
    } catch (e) {
      if (req === reqRef.current) setError(e instanceof Error ? e.message : String(e))
    } finally {
      if (req === reqRef.current) setLoading(false)
    }
  }, [url])

  useEffect(() => {
    load()
    return () => {
      reqRef.current++ // ignore anything still in flight after unmount / filter change
    }
  }, [load])

  return { data, error, loading, reload: load }
}
