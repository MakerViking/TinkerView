import { useState } from 'react'
import { useLive } from '@/store/live'
import { CameraTile } from './CameraTile'
import { AddCameraModal } from '@/components/devices/AddCameraModal'
import { Button, EmptyState, SectionHeader } from '@/components/ui'
import { IconCamera, IconPlus } from '@/components/icons'

export function CameraWall() {
  const cameras = useLive((s) => s.cameras)
  const [adding, setAdding] = useState(false)
  const names = Object.keys(cameras).sort()
  const recording = names.filter((n) => cameras[n]?.recordings).length

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
