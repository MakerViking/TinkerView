// The per-camera HQ-stream preference lives in localStorage as `hq:<name>`
// (see usePersistentBoolean). Renaming a camera must carry it over, or the tile
// silently drops back to the standard stream.
export function migrateHqPref(oldName: string, newName: string): void {
  if (oldName === newName) return
  try {
    const v = window.localStorage.getItem(`hq:${oldName}`)
    if (v !== null) window.localStorage.setItem(`hq:${newName}`, v)
    window.localStorage.removeItem(`hq:${oldName}`)
  } catch {
    /* storage unavailable — nothing to migrate */
  }
}
