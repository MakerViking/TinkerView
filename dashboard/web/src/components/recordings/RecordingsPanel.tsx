import { useMemo, useState } from 'react'
import { useLive } from '@/store/live'
import { Modal } from '@/components/Modal'
import { useFrigateEvents, type FrigateEvent } from './useFrigateEvents'

const LABEL_ICON: Record<string, string> = {
  person: '🚶',
  car: '🚗',
  bicycle: '🚲',
  motorcycle: '🏍️',
  truck: '🚚',
  bus: '🚌',
  cat: '🐱',
  dog: '🐶',
  bird: '🐦',
}

const selectCls =
  'rounded-md bg-slate-800 border border-slate-700 px-2 py-1 text-sm text-slate-100 focus:outline-none focus:border-sky-500'

// RecordingsPanel is a custom browser over Frigate's event clips (the recordings
// produced by object-only recording). It hits Frigate's HTTP API through the
// dashboard's /frigate/ proxy — no dashboard backend involved.
export function RecordingsPanel() {
  const cameras = useLive((s) => s.cameras)
  const cameraNames = useMemo(() => Object.keys(cameras).sort(), [cameras])

  const [camera, setCamera] = useState('')
  const [label, setLabel] = useState('')
  const [playing, setPlaying] = useState<FrigateEvent | null>(null)

  const { data, error, loading, reload } = useFrigateEvents({
    camera: camera || undefined,
    label: label || undefined,
    limit: 100,
  })
  const events = data ?? []
  const labels = useMemo(() => {
    const s = new Set<string>()
    for (const e of events) s.add(e.label)
    return Array.from(s).sort()
  }, [events])

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <h1 className="text-sm font-medium text-slate-300">Recordings</h1>
        <select value={camera} onChange={(e) => setCamera(e.target.value)} className={selectCls}>
          <option value="">All cameras</option>
          {cameraNames.map((c) => (
            <option key={c} value={c}>
              {c}
            </option>
          ))}
        </select>
        <select value={label} onChange={(e) => setLabel(e.target.value)} className={selectCls}>
          <option value="">All objects</option>
          {(labels.length ? labels : ['person', 'car']).map((l) => (
            <option key={l} value={l}>
              {l}
            </option>
          ))}
        </select>
        <button
          onClick={reload}
          title="Refresh"
          className="rounded-md bg-slate-800 border border-slate-700 px-2 py-1 text-sm text-slate-300 hover:bg-slate-700"
        >
          ↻
        </button>
        {loading && <span className="text-xs text-slate-500">loading…</span>}
      </div>

      {error && (
        <div className="rounded-md border border-rose-800 bg-rose-950/40 p-3 text-sm text-rose-200">
          Couldn&apos;t load recordings: {error}. Is Frigate reachable?
        </div>
      )}

      {!error && !loading && !events.length && (
        <div className="p-8 text-center text-slate-400">
          No recordings yet. Clips appear here when a tracked object is detected.
        </div>
      )}

      <div className="grid gap-3 grid-cols-[repeat(auto-fill,minmax(220px,1fr))]">
        {events.map((e) => (
          <EventCard key={e.id} e={e} onPlay={() => setPlaying(e)} />
        ))}
      </div>

      {playing && (
        <Modal
          size="xl"
          title={`${LABEL_ICON[playing.label] ?? '🔔'} ${playing.sub_label || playing.label} · ${playing.camera}`}
          onClose={() => setPlaying(null)}
        >
          {playing.has_clip ? (
            // eslint-disable-next-line jsx-a11y/media-has-caption
            <video
              src={`/frigate/api/events/${playing.id}/clip.mp4`}
              controls
              autoPlay
              className="w-full rounded bg-black"
            />
          ) : (
            <img
              src={`/frigate/api/events/${playing.id}/snapshot.jpg`}
              alt=""
              className="w-full rounded bg-black"
            />
          )}
          <div className="mt-2 text-xs text-slate-400">
            {new Date(playing.start_time * 1000).toLocaleString()}
            {playing.zones?.length ? ` · zones: ${playing.zones.join(', ')}` : ''}
          </div>
        </Modal>
      )}
    </div>
  )
}

function EventCard({ e, onPlay }: { e: FrigateEvent; onPlay: () => void }) {
  return (
    <button
      onClick={onPlay}
      className="group text-left rounded-lg border border-slate-800 bg-slate-900/60 overflow-hidden hover:border-slate-600"
    >
      <div className="relative aspect-video bg-black">
        <img
          src={`/frigate/api/events/${e.id}/thumbnail.jpg`}
          alt=""
          loading="lazy"
          className="w-full h-full object-cover"
        />
        <span className="absolute left-1 top-1 rounded bg-black/70 px-1.5 py-0.5 text-[11px]">
          {(LABEL_ICON[e.label] ?? '🔔') + ' ' + (e.sub_label || e.label)}
        </span>
        {e.has_clip && (
          <span className="absolute right-1 bottom-1 rounded bg-black/70 px-1.5 py-0.5 text-[11px]">▶</span>
        )}
      </div>
      <div className="flex items-center gap-1 px-2 py-1.5 text-xs text-slate-400">
        <span className="truncate">{e.camera}</span>
        <time className="ml-auto whitespace-nowrap">
          {new Date(e.start_time * 1000).toLocaleString([], {
            month: 'short',
            day: 'numeric',
            hour: '2-digit',
            minute: '2-digit',
          })}
        </time>
      </div>
    </button>
  )
}
