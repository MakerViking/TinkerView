import { useWs } from '@/ws/WebSocketProvider'
import { useConfirm } from '@/components/ConfirmDialog'
import { Button, Skeleton } from '@/components/ui'
import { IconRefresh } from '@/components/icons'
import { fmtDuration } from '@/lib/utils'
import { moonrakerUrl, useMoonraker } from './useMoonraker'

interface FileEntry {
  path: string
  modified: number
  size: number
}
interface FileMeta {
  result?: {
    estimated_time?: number
    filament_total?: number
    thumbnails?: { width: number; height: number; relative_path: string }[]
  }
}

// FilesPanel lists the printer's gcode library (most-recent first) with lazy
// thumbnails + metadata, and lets you start/reprint a file via the WS control path.
export function FilesPanel({ id }: { id: string }) {
  const { send } = useWs()
  const { ask, confirmNode } = useConfirm()
  const { data, error, loading, reload } = useMoonraker<{ result: FileEntry[] }>(
    id,
    '/server/files/list?root=gcodes',
  )
  const print = (path: string) =>
    ask({
      title: 'Start print',
      message: `Start printing ${path}?`,
      confirmLabel: 'Print',
      onConfirm: () => send({ type: 'printer.control', data: { id, action: { op: 'print_file', name: path } } }),
    })
  const files = (data?.result ?? []).slice().sort((a, b) => b.modified - a.modified).slice(0, 30)

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-2">
        <Button
          size="xs"
          variant="outline"
          onClick={() =>
            ask({
              title: 'Reprint last file',
              message: 'Reprint the last file?',
              confirmLabel: 'Reprint',
              onConfirm: () => send({ type: 'printer.control', data: { id, action: { op: 'reprint' } } }),
            })
          }
        >
          <IconRefresh className="text-[12px]" />
          Reprint last
        </Button>
        <Button size="xs" variant="ghost" onClick={reload} className="ml-auto">
          Refresh
        </Button>
      </div>
      {loading && !data && (
        <div className="flex flex-col gap-2" aria-label="Loading files">
          <Skeleton className="h-12" />
          <Skeleton className="h-12" />
          <Skeleton className="h-12" />
        </div>
      )}
      {error && <div className="text-xs text-rose-300">Couldn’t load files: {error}</div>}
      <div className="flex flex-col divide-y divide-ink-800">
        {files.map((f) => (
          <FileRow key={f.path} id={id} file={f} onPrint={() => print(f.path)} />
        ))}
        {data && !files.length && <div className="py-3 text-xs text-ink-500">No gcode files.</div>}
      </div>
      {confirmNode}
    </div>
  )
}

function FileRow({ id, file, onPrint }: { id: string; file: FileEntry; onPrint: () => void }) {
  const { data } = useMoonraker<FileMeta>(id, `/server/files/metadata?filename=${encodeURIComponent(file.path)}`)
  const m = data?.result
  const thumb = m?.thumbnails?.slice().sort((a, b) => b.width - a.width)[0]
  const name = file.path.split('/').pop() ?? file.path
  return (
    <div className="flex items-center gap-2.5 py-2">
      <div className="h-10 w-10 shrink-0 overflow-hidden rounded-md border border-ink-800 bg-ink-850">
        {thumb && (
          <img src={thumbUrl(id, file.path, thumb.relative_path)} alt="" className="h-full w-full object-cover" loading="lazy" />
        )}
      </div>
      <div className="min-w-0 flex-1">
        <div className="truncate font-mono text-xs text-ink-100">{name}</div>
        <div className="font-mono text-[11px] text-ink-500">
          {m?.estimated_time ? fmtDuration(m.estimated_time) : ''}
          {m?.filament_total ? ` · ${(m.filament_total / 1000).toFixed(1)} m` : ''}
        </div>
      </div>
      <Button size="xs" variant="primary" onClick={onPrint}>
        Print
      </Button>
    </div>
  )
}

function thumbUrl(id: string, filePath: string, relPath: string): string {
  const slash = filePath.lastIndexOf('/')
  const dir = slash >= 0 ? filePath.slice(0, slash + 1) : ''
  return moonrakerUrl(id, '/server/files/gcodes/' + encodeURI(dir + relPath))
}
