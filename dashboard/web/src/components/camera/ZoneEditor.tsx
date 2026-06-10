import { useRef, useState, type MouseEvent } from 'react'
import type { Zone } from '@/types/ws'
import { inputCls } from '@/components/devices/forms'
import { chipCls } from './CameraSettingsModal'
import { Button } from '@/components/ui'
import { IconPlus, IconTrash } from '@/components/icons'
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
              'rounded-md px-2 py-1 font-display text-xs font-medium tracking-wide transition-colors',
              i === selected
                ? 'bg-brand-500/15 text-brand-300 ring-1 ring-inset ring-brand-400/30'
                : 'bg-ink-800 text-ink-300 hover:bg-ink-700',
            )}
          >
            {z.name || `zone ${i + 1}`}
          </button>
        ))}
        <Button type="button" size="xs" variant="outline" onClick={addZone}>
          <IconPlus className="text-[12px]" />
          Zone
        </Button>
      </div>

      <div
        className="relative w-full overflow-hidden rounded-xl border border-ink-800 bg-black"
        style={{ aspectRatio: '16 / 9' }}
      >
        <img
          ref={imgRef}
          src={`/go2rtc/api/frame.jpeg?src=${encodeURIComponent(cameraName)}`}
          alt={`Live frame of ${cameraName}`}
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
                  className={i === selected ? 'fill-brand-400/25 stroke-brand-300' : 'fill-ink-400/10 stroke-ink-400'}
                  strokeWidth={0.004}
                />
              ),
          )}
          {zone?.coordinates.map((p, i) => <circle key={i} cx={p[0]} cy={p[1]} r={0.008} className="fill-brand-200" />)}
        </svg>
      </div>

      {zone ? (
        <div className="flex flex-col gap-2 rounded-xl border border-ink-800 bg-ink-950/40 p-3">
          <div className="flex items-center gap-2">
            <input
              value={zone.name}
              onChange={(e) => update(selected, { name: e.target.value })}
              placeholder="zone name"
              aria-label="Zone name"
              className={cn(inputCls, '!mt-0')}
            />
            <Button type="button" size="xs" variant="soft" onClick={() => update(selected, { coordinates: [] })}>
              Clear points
            </Button>
            <Button type="button" size="xs" variant="danger-soft" onClick={() => removeZone(selected)}>
              <IconTrash className="text-[12px]" />
              Delete
            </Button>
          </div>
          <div className="flex flex-wrap gap-1.5">
            {OBJECT_OPTS.map((o) => (
              <button
                type="button"
                key={o}
                onClick={() => toggleObj(o)}
                className={chipCls(!!zone.objects?.includes(o))}
              >
                {o}
              </button>
            ))}
          </div>
          <p className="text-[11px] text-ink-500">
            Click the image to add polygon points ({zone.coordinates.length}). Only the selected object types inside
            the zone trigger alerts/recording.
          </p>
        </div>
      ) : (
        <p className="text-[11px] text-ink-500">
          No zones — the whole frame is watched. Add a zone to limit detection to an area of interest.
        </p>
      )}
    </div>
  )
}
