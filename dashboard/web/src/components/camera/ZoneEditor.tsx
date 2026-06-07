import { useRef, useState, type MouseEvent } from 'react'
import type { Zone } from '@/types/ws'
import { inputCls } from '@/components/devices/forms'
import { cn } from '@/lib/utils'

const OBJECT_OPTS = ['person', 'car', 'bicycle', 'motorcycle', 'truck', 'cat', 'dog', 'bird']
const round3 = (n: number) => Math.round(n * 1000) / 1000

// ZoneEditor draws Frigate detection zones (areas of interest) as polygons over a
// live snapshot. Click the image to add polygon points; coordinates are normalized
// 0..1 to the frame. Objects of the selected types inside a zone trigger
// alerts/recording (the whole frame is watched when there are no zones).
export function ZoneEditor({
  cameraName,
  zones,
  onChange,
}: {
  cameraName: string
  zones: Zone[]
  onChange: (zones: Zone[]) => void
}) {
  const [selected, setSelected] = useState(0)
  const imgRef = useRef<HTMLImageElement>(null)
  const zone = zones[selected] as Zone | undefined

  const update = (i: number, patch: Partial<Zone>) =>
    onChange(zones.map((z, j) => (j === i ? { ...z, ...patch } : z)))

  const addZone = () => {
    onChange([...zones, { name: `zone ${zones.length + 1}`, coordinates: [], objects: ['person', 'car'] }])
    setSelected(zones.length)
  }
  const removeZone = (i: number) => {
    onChange(zones.filter((_, j) => j !== i))
    setSelected(0)
  }
  const onImgClick = (e: MouseEvent) => {
    if (!zone || !imgRef.current) return
    const r = imgRef.current.getBoundingClientRect()
    const x = round3(Math.min(1, Math.max(0, (e.clientX - r.left) / r.width)))
    const y = round3(Math.min(1, Math.max(0, (e.clientY - r.top) / r.height)))
    update(selected, { coordinates: [...zone.coordinates, [x, y]] })
  }
  const toggleObj = (o: string) => {
    if (!zone) return
    const on = zone.objects?.includes(o)
    update(selected, { objects: on ? zone.objects!.filter((x) => x !== o) : [...(zone.objects ?? []), o] })
  }

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center gap-1.5">
        {zones.map((z, i) => (
          <button
            type="button"
            key={i}
            onClick={() => setSelected(i)}
            className={cn(
              'rounded px-2 py-0.5 text-xs',
              i === selected ? 'bg-sky-500/30 text-sky-100' : 'bg-slate-700 text-slate-300',
            )}
          >
            {z.name || `zone ${i + 1}`}
          </button>
        ))}
        <button
          type="button"
          onClick={addZone}
          className="rounded px-2 py-0.5 text-xs bg-slate-700 text-slate-200 hover:bg-slate-600"
        >
          + Zone
        </button>
      </div>

      <div className="relative w-full overflow-hidden rounded bg-black" style={{ aspectRatio: '16 / 9' }}>
        <img
          ref={imgRef}
          src={`/go2rtc/api/frame.jpeg?src=${encodeURIComponent(cameraName)}`}
          alt=""
          draggable={false}
          onClick={onImgClick}
          className="absolute inset-0 h-full w-full cursor-crosshair object-cover select-none"
        />
        <svg className="pointer-events-none absolute inset-0 h-full w-full" viewBox="0 0 1 1" preserveAspectRatio="none">
          {zones.map(
            (z, i) =>
              z.coordinates.length >= 2 && (
                <polygon
                  key={i}
                  points={z.coordinates.map((p) => p.join(',')).join(' ')}
                  className={i === selected ? 'fill-sky-400/25 stroke-sky-300' : 'fill-slate-400/10 stroke-slate-400'}
                  strokeWidth={0.004}
                />
              ),
          )}
          {zone?.coordinates.map((p, i) => <circle key={i} cx={p[0]} cy={p[1]} r={0.008} className="fill-sky-200" />)}
        </svg>
      </div>

      {zone ? (
        <div className="flex flex-col gap-2 rounded border border-slate-800 p-2">
          <div className="flex items-center gap-2">
            <input
              value={zone.name}
              onChange={(e) => update(selected, { name: e.target.value })}
              placeholder="zone name"
              className={cn(inputCls, '!mt-0')}
            />
            <button
              type="button"
              onClick={() => update(selected, { coordinates: [] })}
              className="whitespace-nowrap rounded px-2 py-1 text-xs bg-slate-700 text-slate-200 hover:bg-slate-600"
            >
              Clear points
            </button>
            <button
              type="button"
              onClick={() => removeZone(selected)}
              className="whitespace-nowrap rounded px-2 py-1 text-xs bg-rose-600/80 text-white hover:bg-rose-500"
            >
              Delete
            </button>
          </div>
          <div className="flex flex-wrap gap-1.5">
            {OBJECT_OPTS.map((o) => (
              <button
                type="button"
                key={o}
                onClick={() => toggleObj(o)}
                className={cn(
                  'rounded px-2 py-0.5 text-xs',
                  zone.objects?.includes(o) ? 'bg-emerald-500/30 text-emerald-200' : 'bg-slate-700 text-slate-400',
                )}
              >
                {o}
              </button>
            ))}
          </div>
          <p className="text-[11px] text-slate-500">
            Click the image to add polygon points ({zone.coordinates.length}). Only the selected object types inside
            the zone trigger alerts/recording.
          </p>
        </div>
      ) : (
        <p className="text-[11px] text-slate-500">
          No zones — the whole frame is watched. Add a zone to limit detection to an area of interest.
        </p>
      )}
    </div>
  )
}
