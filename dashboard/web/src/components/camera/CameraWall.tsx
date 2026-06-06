import { useState } from 'react'
import { useLive } from '@/store/live'
import { CameraTile } from './CameraTile'
import { AddCameraModal } from '@/components/devices/AddCameraModal'

export function CameraWall() {
  const cameras = useLive((s) => s.cameras)
  const [adding, setAdding] = useState(false)
  const names = Object.keys(cameras).sort()

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center">
        <h1 className="text-sm font-medium text-slate-300">Cameras</h1>
        <button
          onClick={() => setAdding(true)}
          className="ml-auto rounded-md bg-sky-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-sky-500"
        >
          + Add camera
        </button>
      </div>

      {names.length ? (
        <div className="grid gap-3 grid-cols-[repeat(auto-fit,minmax(320px,1fr))]">
          {names.map((n) => (
            <CameraTile key={n} name={n} />
          ))}
        </div>
      ) : (
        <div className="p-8 text-center text-slate-400">
          No cameras yet. Add one above, or they appear once the Frigate adapter is connected.
        </div>
      )}

      {adding && <AddCameraModal onClose={() => setAdding(false)} />}
    </div>
  )
}
