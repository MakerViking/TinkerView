import { useState } from 'react'
import { WebSocketProvider } from '@/ws/WebSocketProvider'
import { AppShell, type Tab } from '@/components/AppShell'
import { ErrorBoundary } from '@/components/ErrorBoundary'
import { CameraWall } from '@/components/camera/CameraWall'
import { RecordingsPanel } from '@/components/recordings/RecordingsPanel'
import { PrinterPanel } from '@/components/printer/PrinterPanel'
import { RecognitionPanel } from '@/components/recognition/RecognitionPanel'
import { DetectionPanel } from '@/components/detection/DetectionPanel'
import { HealthPanel } from '@/components/health/HealthPanel'
import { AlertsFeed } from '@/components/alerts/AlertsFeed'
import { AddPrinterModal } from '@/components/devices/AddPrinterModal'
import { Button, EmptyState, SectionHeader } from '@/components/ui'
import { IconPlus, IconPrinter3d } from '@/components/icons'
import { useLive } from '@/store/live'

export default function App() {
  const [tab, setTab] = useState<Tab>('cameras')
  return (
    <WebSocketProvider>
      <AppShell tab={tab} onTab={setTab}>
        <ErrorBoundary key={tab}>
          {tab === 'cameras' && <CameraWall />}
          {tab === 'recordings' && <RecordingsPanel />}
          {tab === 'printers' && <PrintersView />}
          {tab === 'recognition' && <RecognitionPanel />}
          {tab === 'detection' && <DetectionPanel />}
          {tab === 'health' && <HealthPanel />}
          {tab === 'alerts' && <AlertsFeed />}
        </ErrorBoundary>
      </AppShell>
    </WebSocketProvider>
  )
}

function PrintersView() {
  const printers = useLive((s) => s.printers)
  const [adding, setAdding] = useState(false)
  const ids = Object.keys(printers).sort()
  const printing = ids.filter((id) => printers[id]?.state === 'printing').length
  return (
    <div className="flex flex-col gap-4">
      <SectionHeader
        title="Printers"
        meta={ids.length ? `${ids.length} · ${printing} printing` : undefined}
      >
        <Button variant="primary" onClick={() => setAdding(true)}>
          <IconPlus className="text-[14px]" />
          Add printer
        </Button>
      </SectionHeader>
      {ids.length ? (
        <div className="grid gap-4 grid-cols-[repeat(auto-fit,minmax(340px,1fr))]">
          {ids.map((id) => (
            <PrinterPanel key={id} id={id} />
          ))}
        </div>
      ) : (
        <EmptyState
          icon={<IconPrinter3d />}
          title="No printers yet"
          hint="Connect a Klipper, OctoPrint, Prusa or Bambu Lab printer to monitor and control it from here."
          action={
            <Button variant="primary" onClick={() => setAdding(true)}>
              <IconPlus className="text-[14px]" />
              Add your first printer
            </Button>
          }
        />
      )}
      {adding && <AddPrinterModal onClose={() => setAdding(false)} />}
    </div>
  )
}
