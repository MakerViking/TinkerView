import { useState } from 'react'
import { SectionHeader, Segmented } from '@/components/ui'
import { FacesPanel } from './FacesPanel'
import { PlatesPanel } from './PlatesPanel'
import { RulesPanel } from './RulesPanel'

type Sub = 'faces' | 'plates' | 'rules'

// RecognitionPanel groups the AI-recognition enrollment (teach Frigate which faces
// and plates are "known") with the notification rules that turn those recognitions
// into prioritized phone pushes.
export function RecognitionPanel() {
  const [sub, setSub] = useState<Sub>('faces')
  return (
    <div className="flex flex-col gap-4">
      <SectionHeader title="Recognition">
        <Segmented
          value={sub}
          onChange={setSub}
          options={[
            { value: 'faces', label: 'Faces' },
            { value: 'plates', label: 'License plates' },
            { value: 'rules', label: 'Notification rules' },
          ]}
        />
      </SectionHeader>
      {sub === 'faces' && <FacesPanel />}
      {sub === 'plates' && <PlatesPanel />}
      {sub === 'rules' && <RulesPanel />}
    </div>
  )
}
