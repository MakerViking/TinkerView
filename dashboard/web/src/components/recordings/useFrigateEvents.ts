import { useCallback, useEffect, useState } from 'react'

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
  const [loading, setLoading] = useState(false)
  const url = frigateEventsUrl(f)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const r = await fetch(url)
      if (!r.ok) throw new Error(`HTTP ${r.status}`)
      setData((await r.json()) as FrigateEvent[])
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }, [url])

  useEffect(() => {
    load()
  }, [load])

  return { data, error, loading, reload: load }
}
