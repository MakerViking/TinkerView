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
  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center">
        <h1 className="text-sm font-medium text-slate-300">Printers</h1>
        <button
          onClick={() => setAdding(true)}
          className="ml-auto rounded-md bg-sky-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-sky-500"
        >
          + Add printer
        </button>
      </div>
      {ids.length ? (
        <div className="grid gap-4 grid-cols-[repeat(auto-fit,minmax(340px,1fr))]">
          {ids.map((id) => (
            <PrinterPanel key={id} id={id} />
          ))}
        </div>
      ) : (
        <div className="p-8 text-center text-slate-400">No printers configured yet. Add one above.</div>
      )}
      {adding && <AddPrinterModal onClose={() => setAdding(false)} />}
    </div>
  )
}
