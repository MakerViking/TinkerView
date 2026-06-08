import { useCallback, useEffect, useRef, useState } from 'react'
import { inputCls } from '@/components/devices/forms'
import { ConfirmDialog } from '@/components/ConfirmDialog'

// Faces are managed directly through Frigate's API via the dashboard's /frigate/
// reverse proxy (same pattern as the Recordings tab). Frigate stores each person's
// training images under clips/faces/<name>/, served back over the proxy.
type FaceMap = Record<string, string[]>

const FACES_API = '/frigate/api/faces'
const faceImg = (name: string, file: string) =>
  `/frigate/clips/faces/${encodeURIComponent(name)}/${encodeURIComponent(file)}`

// toJpeg decodes an uploaded image with the browser (honoring EXIF orientation —
// important so a sideways phone photo's face is upright) and re-encodes it as a
// clean JPEG that Frigate's OpenCV can always read. Frigate returns a raw 500
// ("'NoneType' has no attribute 'shape'") when cv2 can't decode the bytes — e.g.
// HEIC/AVIF or odd encodings — so normalizing here avoids that. Large images are
// capped at 2048px. Throws a clear message when the browser itself can't decode
// the format (e.g. HEIC in Chrome).
async function toJpeg(file: File): Promise<Blob> {
  let bitmap: ImageBitmap
  try {
    bitmap = await createImageBitmap(file, { imageOrientation: 'from-image' })
  } catch {
    throw new Error(`couldn't read "${file.name}" — use a JPG or PNG (HEIC isn't supported)`)
  }
  const maxDim = 2048
  const scale = Math.min(1, maxDim / Math.max(bitmap.width, bitmap.height))
  const w = Math.max(1, Math.round(bitmap.width * scale))
  const h = Math.max(1, Math.round(bitmap.height * scale))
  const canvas = document.createElement('canvas')
  canvas.width = w
  canvas.height = h
  const ctx = canvas.getContext('2d')
  if (!ctx) throw new Error('canvas not available')
  ctx.drawImage(bitmap, 0, 0, w, h)
  bitmap.close()
  const blob = await new Promise<Blob | null>((res) => canvas.toBlob(res, 'image/jpeg', 0.92))
  if (!blob) throw new Error('failed to encode the image as JPEG')
  return blob
}

export function FacesPanel() {
  const [faces, setFaces] = useState<FaceMap | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [disabled, setDisabled] = useState(false)
  const [newName, setNewName] = useState('')
  const [confirm, setConfirm] = useState<{ name: string } | null>(null)

  const load = useCallback(async () => {
    setError(null)
    try {
      const r = await fetch(FACES_API)
      if (r.status === 404) {
        setDisabled(true)
        return
      }
      if (!r.ok) throw new Error(`HTTP ${r.status}`)
      setFaces((await r.json()) as FaceMap)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const createPerson = async () => {
    const name = newName.trim()
    if (!name) return
    setError(null)
    try {
      const r = await fetch(`${FACES_API}/${encodeURIComponent(name)}/create`, { method: 'POST' })
      if (!r.ok) throw new Error(`create failed: HTTP ${r.status}`)
      setNewName('')
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }

  const removePerson = async (name: string) => {
    const files = faces?.[name] ?? []
    setError(null)
    try {
      // Frigate has no "delete face" endpoint — remove all of the person's images.
      const r = await fetch(`${FACES_API}/${encodeURIComponent(name)}/delete`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids: files }),
      })
      if (!r.ok) throw new Error(`delete failed: HTTP ${r.status}`)
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }

  if (disabled) {
    return (
      <Notice>
        Face recognition isn't enabled in Frigate. Add <code className="text-slate-300">face_recognition:
        enabled: true</code> to the Frigate config and restart, then enroll faces here.
      </Notice>
    )
  }

  const names = faces ? Object.keys(faces).sort() : []
  return (
    <div className="flex flex-col gap-4">
      <p className="text-xs text-slate-500">
        Teach Frigate which faces are known. Recognized people appear by name on detections and can be
        prioritized (or silenced) under <span className="text-slate-300">Notification rules</span>. Upload a
        few clear, well-lit shots per person from different angles.
      </p>
      <div className="flex items-end gap-2">
        <label className="block flex-1 max-w-xs">
          <span className="text-xs text-slate-400">Add a person</span>
          <input
            className={inputCls}
            placeholder="e.g. Thomas"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && createPerson()}
          />
        </label>
        <button
          onClick={createPerson}
          disabled={!newName.trim()}
          className="rounded-md bg-sky-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-sky-500 disabled:opacity-50"
        >
          Add person
        </button>
      </div>

      {error && <Notice tone="error">{error}</Notice>}

      {names.length === 0 ? (
        <div className="p-8 text-center text-slate-400">No faces enrolled yet. Add a person above.</div>
      ) : (
        <div className="grid gap-3 grid-cols-[repeat(auto-fill,minmax(260px,1fr))]">
          {names.map((name) => (
            <PersonCard
              key={name}
              name={name}
              files={faces?.[name] ?? []}
              onChanged={load}
              onRemove={() => setConfirm({ name })}
              onError={setError}
            />
          ))}
        </div>
      )}

      {confirm && (
        <ConfirmDialog
          title={`Remove ${confirm.name}?`}
          message="This deletes all of this person's enrolled face images from Frigate."
          confirmLabel="Remove"
          danger
          onConfirm={() => removePerson(confirm.name)}
          onClose={() => setConfirm(null)}
        />
      )}
    </div>
  )
}

function PersonCard({
  name,
  files,
  onChanged,
  onRemove,
  onError,
}: {
  name: string
  files: string[]
  onChanged: () => void
  onRemove: () => void
  onError: (m: string) => void
}) {
  const fileRef = useRef<HTMLInputElement>(null)
  const [busy, setBusy] = useState(false)

  const upload = async (list: FileList | null) => {
    if (!list || list.length === 0) return
    setBusy(true)
    onError('')
    try {
      for (const f of Array.from(list)) {
        const jpeg = await toJpeg(f) // normalize so Frigate's OpenCV can always decode it
        const fd = new FormData()
        fd.append('file', jpeg, 'face.jpg')
        const r = await fetch(`${FACES_API}/${encodeURIComponent(name)}/register`, {
          method: 'POST',
          body: fd,
        })
        if (!r.ok) {
          const body = await r.text().catch(() => '')
          throw new Error(
            body && body.length < 200 ? `upload failed: ${body}` : `upload failed: HTTP ${r.status}`,
          )
        }
      }
      onChanged()
    } catch (e) {
      onError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
      if (fileRef.current) fileRef.current.value = ''
    }
  }

  const deleteImage = async (file: string) => {
    onError('')
    try {
      const r = await fetch(`${FACES_API}/${encodeURIComponent(name)}/delete`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ids: [file] }),
      })
      if (!r.ok) throw new Error(`delete failed: HTTP ${r.status}`)
      onChanged()
    } catch (e) {
      onError(e instanceof Error ? e.message : String(e))
    }
  }

  return (
    <div className="rounded-lg border border-slate-800 bg-slate-900/60 p-3">
      <div className="flex items-center gap-2">
        <span className="font-medium text-slate-200">{name}</span>
        <span className="text-xs text-slate-500">{files.length} image{files.length === 1 ? '' : 's'}</span>
        <button
          onClick={onRemove}
          className="ml-auto text-xs text-rose-400 hover:text-rose-300"
          title="Remove person"
        >
          Remove
        </button>
      </div>
      {files.length > 0 && (
        <div className="mt-2 flex flex-wrap gap-1.5">
          {files.map((f) => (
            <div key={f} className="group relative">
              <img
                src={faceImg(name, f)}
                alt={f}
                className="h-14 w-14 rounded object-cover bg-black"
                loading="lazy"
              />
              <button
                onClick={() => deleteImage(f)}
                className="absolute -right-1 -top-1 hidden h-4 w-4 items-center justify-center rounded-full bg-rose-600 text-[10px] leading-none text-white group-hover:flex"
                title="Delete image"
              >
                ✕
              </button>
            </div>
          ))}
        </div>
      )}
      <button
        onClick={() => fileRef.current?.click()}
        disabled={busy}
        className="mt-2 w-full rounded-md border border-dashed border-slate-700 px-2 py-1.5 text-xs text-slate-400 hover:border-sky-600 hover:text-sky-300 disabled:opacity-50"
      >
        {busy ? 'Uploading…' : '+ Upload face image(s)'}
      </button>
      <input
        ref={fileRef}
        type="file"
        accept="image/*"
        multiple
        className="hidden"
        onChange={(e) => upload(e.target.files)}
      />
    </div>
  )
}

function Notice({ children, tone }: { children: React.ReactNode; tone?: 'error' }) {
  return (
    <div
      className={
        'rounded-lg border p-3 text-sm ' +
        (tone === 'error'
          ? 'border-rose-800 bg-rose-950/40 text-rose-200'
          : 'border-slate-800 bg-slate-900/60 text-slate-300')
      }
    >
      {children}
    </div>
  )
}
