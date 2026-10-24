import { useMemo, useState } from 'react'
import { useLive } from '@/store/live'
import { Modal } from '@/components/Modal'
import { EmptyState, Notice, SectionHeader, Skeleton } from '@/components/ui'
import { IconFilm, IconPlay, IconRefresh } from '@/components/icons'
import { useFrigateEvents, type FrigateEvent } from './useFrigateEvents'
import { withToken } from '@/lib/auth'

const selectCls =
  'rounded-md border border-ink-800 bg-ink-950 px-3 py-1.5 text-sm text-ink-100 ' +
  'focus:border-ink-600 focus:outline-none'

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
    <div className="flex flex-col gap-6">
      <SectionHeader title="Recordings" meta={events.length ? `${events.length} events` : undefined}>
        <select
          value={camera}
          onChange={(e) => setCamera(e.target.value)}
          aria-label="Filter by camera"
          className={selectCls}
        >
          <option value="">All cameras</option>
          {cameraNames.map((c) => (
            <option key={c} value={c}>
              {c}
            </option>
          ))}
        </select>
        <select
          value={label}
          onChange={(e) => setLabel(e.target.value)}
          aria-label="Filter by object"
          className={selectCls}
        >
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
          aria-label="Refresh recordings"
          className="grid h-8 w-8 place-items-center rounded-md border border-ink-800 bg-ink-950 text-ink-300 transition-colors hover:border-ink-600 hover:text-ink-100"
        >
          <IconRefresh className="text-[14px]" />
        </button>
      </SectionHeader>

      {error && <Notice tone="error">Couldn&apos;t load recordings: {error}. Is Frigate reachable?</Notice>}

      {loading && !events.length && !error && (
        <div className="grid gap-3 grid-cols-[repeat(auto-fill,minmax(220px,1fr))]" aria-label="Loading recordings">
          {Array.from({ length: 8 }).map((_, i) => (
            <Skeleton key={i} className="aspect-[16/11]" />
          ))}
        </div>
      )}

      {!error && !loading && !events.length && (
        <EmptyState
          icon={<IconFilm />}
          title="No recordings yet"
          hint="Clips appear here when a tracked object is detected on a recording camera."
        />
      )}

      <div className="grid gap-3 grid-cols-[repeat(auto-fill,minmax(220px,1fr))]">
        {events.map((e) => (
          <EventCard key={e.id} e={e} onPlay={() => setPlaying(e)} />
        ))}
      </div>

      {playing && (
        <Modal
          size="xl"
          title={`${playing.sub_label || playing.label} · ${playing.camera}`}
          onClose={() => setPlaying(null)}
        >
          {playing.has_clip ? (
            // eslint-disable-next-line jsx-a11y/media-has-caption
            <video
              src={withToken(`/frigate/api/events/${playing.id}/clip.mp4`)}
              controls
              autoPlay
              className="w-full rounded-lg bg-black"
            />
          ) : (
            <img
              src={withToken(`/frigate/api/events/${playing.id}/snapshot.jpg`)}
              alt=""
              className="w-full rounded-lg bg-black"
            />
          )}
          <div className="mt-2.5 text-[13px] text-ink-400">
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
    <button onClick={onPlay} className="group overflow-hidden rounded-lg bg-ink-900 text-left">
      <div className="relative aspect-video bg-black">
        <img
          src={withToken(`/frigate/api/events/${e.id}/thumbnail.jpg`)}
          alt={`${e.label} on ${e.camera}`}
          loading="lazy"
          className="h-full w-full object-cover"
        />
        <span className="absolute left-1.5 top-1.5 rounded-md bg-black/55 px-1.5 py-0.5 text-[12px] text-white">
          {e.sub_label || e.label}
        </span>
        {e.has_clip && (
          <span className="absolute bottom-1.5 right-1.5 grid h-6 w-6 place-items-center rounded-full bg-black/55 text-white opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100 pointer-coarse:opacity-100">
            <IconPlay className="text-[11px]" />
          </span>
        )}
      </div>
      <div className="flex items-center gap-2 px-2.5 py-2">
        <span className="truncate text-[13px] text-ink-200">{e.camera}</span>
        <time className="ml-auto whitespace-nowrap text-[12px] tabular-nums text-ink-500">
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
