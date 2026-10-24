import { useCallback, useEffect, useRef, useState } from 'react'
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
  // reqRef ids the latest request so a stale response is ignored; keyRef is the
  // id/path `data` belongs to.
  const reqRef = useRef(0)
  const keyRef = useRef(`${id}|${path}`)

  const load = useCallback(async () => {
    const req = ++reqRef.current
    const key = `${id}|${path}`
    if (keyRef.current !== key) {
      keyRef.current = key
      setData(null) // different printer/path: don't show the previous one's data
    }
    if (!path) {
      // The cleanup bumped reqRef, so any in-flight request's finally is ignored;
      // reset here or loading/error would stick for the null path.
      setLoading(false)
      setError(null)
      return
    }
    setLoading(true)
    setError(null)
    try {
      const r = await fetch(moonrakerUrl(id, path))
      if (!r.ok) throw new Error(`HTTP ${r.status}`)
      const d = (await r.json()) as T
      if (req === reqRef.current) setData(d)
    } catch (e) {
      if (req === reqRef.current) setError(e instanceof Error ? e.message : String(e))
    } finally {
      if (req === reqRef.current) setLoading(false)
    }
  }, [id, path])

  useEffect(() => {
    load()
    return () => {
      reqRef.current++
    }
  }, [load])

  return { data, error, loading, reload: load }
}
