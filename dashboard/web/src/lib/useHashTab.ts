import { useCallback, useEffect, useState } from 'react'

// useHashTab keeps the active tab in location.hash (#printers, #health…), so a
// reload stays on the same view, tabs can be bookmarked, and Back/Forward move
// between tabs. Unknown or empty hashes fall back to `fallback`.
export function useHashTab<T extends string>(valid: readonly T[], fallback: T): [T, (t: T) => void] {
  const read = useCallback((): T => {
    const h = decodeURIComponent(location.hash.slice(1)) as T
    return valid.includes(h) ? h : fallback
  }, [valid, fallback])

  const [tab, setTab] = useState<T>(read)

  useEffect(() => {
    const onHash = () => setTab(read())
    window.addEventListener('hashchange', onHash)
    return () => window.removeEventListener('hashchange', onHash)
  }, [read])

  const go = useCallback(
    (t: T) => {
      if (t === read()) return
      // The default tab gets a clean URL rather than a trailing "#cameras".
      if (t === fallback) history.pushState(null, '', location.pathname + location.search)
      else location.hash = t
      setTab(t)
    },
    [read, fallback],
  )

  return [tab, go]
}
