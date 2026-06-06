import { useCallback, useState } from 'react'

// usePersistentBoolean is a drop-in useState<boolean> whose value is mirrored to
// localStorage under `key`, so per-tile UI preferences (e.g. the camera HQ
// toggle) survive reloads. localStorage access is wrapped because it throws when
// storage is unavailable (private mode / blocked cookies); there we silently
// degrade to plain in-memory state for the session.
export function usePersistentBoolean(
  key: string,
  fallback = false,
): [boolean, (v: boolean | ((prev: boolean) => boolean)) => void] {
  const [value, setValue] = useState<boolean>(() => {
    try {
      const raw = window.localStorage.getItem(key)
      return raw === null ? fallback : raw === '1'
    } catch {
      return fallback
    }
  })

  const set = useCallback(
    (v: boolean | ((prev: boolean) => boolean)) => {
      setValue((prev) => {
        const next = typeof v === 'function' ? v(prev) : v
        try {
          window.localStorage.setItem(key, next ? '1' : '0')
        } catch {
          /* storage unavailable — keep the value in memory only */
        }
        return next
      })
    },
    [key],
  )

  return [value, set]
}
