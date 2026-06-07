import { useState } from 'react'
import { cn } from '@/lib/utils'
import { FacesPanel } from './FacesPanel'
import { PlatesPanel } from './PlatesPanel'
import { RulesPanel } from './RulesPanel'

type Sub = 'faces' | 'plates' | 'rules'

const SUBS: { id: Sub; label: string }[] = [
  { id: 'faces', label: 'Faces' },
  { id: 'plates', label: 'License plates' },
  { id: 'rules', label: 'Notification rules' },
]

// RecognitionPanel groups the AI-recognition enrollment (teach Frigate which faces
// and plates are "known") with the notification rules that turn those recognitions
// into prioritized phone pushes.
export function RecognitionPanel() {
  const [sub, setSub] = useState<Sub>('faces')
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <h1 className="text-sm font-medium text-slate-300">Recognition</h1>
        <div className="ml-auto flex gap-1 rounded-lg bg-slate-900/60 p-1">
          {SUBS.map((s) => (
            <button
              key={s.id}
              onClick={() => setSub(s.id)}
              className={cn(
                'rounded-md px-3 py-1 text-sm transition',
                sub === s.id ? 'bg-sky-500/20 text-sky-300' : 'text-slate-400 hover:text-slate-200',
              )}
            >
              {s.label}
            </button>
          ))}
        </div>
      </div>
      {sub === 'faces' && <FacesPanel />}
      {sub === 'plates' && <PlatesPanel />}
      {sub === 'rules' && <RulesPanel />}
    </div>
  )
}
