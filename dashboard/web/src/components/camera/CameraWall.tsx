import { useState } from 'react'
import { useShallow } from 'zustand/react/shallow'
import { useLive } from '@/store/live'
import { CameraTile } from './CameraTile'
import { AddCameraModal } from '@/components/devices/AddCameraModal'
import { Button, EmptyState, SectionHeader } from '@/components/ui'
import { IconCamera, IconPlus } from '@/components/icons'

export function CameraWall() {
  // Subscribe to the sorted name list only (shallow-equal, so a camera.state delta
  // that doesn't add/remove a camera keeps the same value) plus a numeric recording
  // count; each CameraTile subscribes to its own camera.
  const names = useLive(useShallow((s) => Object.keys(s.cameras).sort()))
  const recording = useLive((s) => Object.values(s.cameras).filter((c) => c.recordings).length)
  const [adding, setAdding] = useState(false)

  return (
    <div className="flex flex-col gap-3">
      <SectionHeader
        title="Cameras"
        meta={names.length ? `${names.length} · ${recording} recording` : undefined}
      >
        <Button variant="primary" onClick={() => setAdding(true)}>
          <IconPlus className="text-[14px]" />
          Add camera
        </Button>
      </SectionHeader>

      {names.length ? (
        <div className="grid gap-3 grid-cols-[repeat(auto-fit,minmax(320px,1fr))]">
          {names.map((n) => (
            <CameraTile key={n} name={n} />
          ))}
        </div>
      ) : (
        <EmptyState
          icon={<IconCamera />}
          title="No cameras yet"
          hint="Add one here, or cameras appear automatically once the Frigate adapter is connected."
          action={
            <Button variant="primary" onClick={() => setAdding(true)}>
              <IconPlus className="text-[14px]" />
              Add your first camera
            </Button>
          }
        />
      )}

      {adding && <AddCameraModal onClose={() => setAdding(false)} />}
    </div>
  )
}
