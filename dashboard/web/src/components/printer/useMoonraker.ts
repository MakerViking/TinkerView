import { useCallback, useEffect, useState } from 'react'
import { withToken } from '@/lib/auth'

// moonrakerUrl builds a same-origin URL through the dashboard's read-only
// per-printer Moonraker proxy (file browse / thumbnails / history). It carries the
// shared token as ?token= so <img> thumbnails (which can't set an auth header) work
// when DASHBOARD_TOKEN is configured; a no-op when it isn't.
export function moonrakerUrl(id: string, path: string): string {
  return withToken(`/api/printers/${encodeURIComponent(id)}/moonraker${path}`)
}

// useMoonraker fetches JSON from a printer's Moonraker via the proxy. Pass a null
// path to skip (e.g. when a tab isn't open yet).
export function useMoonraker<T>(id: string, path: string | null) {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  const load = useCallback(async () => {
    if (!path) return
    setLoading(true)
    setError(null)
    try {
      const r = await fetch(moonrakerUrl(id, path))
      if (!r.ok) throw new Error(`HTTP ${r.status}`)
      setData((await r.json()) as T)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }, [id, path])

  useEffect(() => {
    load()
  }, [load])

  return { data, error, loading, reload: load }
}
