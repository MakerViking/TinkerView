import { useWs } from '@/ws/WebSocketProvider'
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

const btn =
  'rounded px-2 py-1 text-xs bg-sky-600 text-white hover:bg-sky-500 disabled:opacity-40'

// FilesPanel lists the printer's gcode library (most-recent first) with lazy
// thumbnails + metadata, and lets you start/reprint a file via the WS control path.
export function FilesPanel({ id }: { id: string }) {
  const { send } = useWs()
  const { data, error, loading, reload } = useMoonraker<{ result: FileEntry[] }>(
    id,
    '/server/files/list?root=gcodes',
  )
  const print = (path: string) => {
    if (confirm(`Start printing ${path}?`)) send({ type: 'printer.control', data: { id, action: { op: 'print_file', name: path } } })
  }
  const files = (data?.result ?? []).slice().sort((a, b) => b.modified - a.modified).slice(0, 30)

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-2">
        <button
          onClick={() => {
            if (confirm('Reprint the last file?')) send({ type: 'printer.control', data: { id, action: { op: 'reprint' } } })
          }}
          className="rounded px-2 py-1 text-xs bg-slate-700/70 text-slate-200 hover:bg-slate-600"
        >
          ↻ Reprint last
        </button>
        <button onClick={reload} className="ml-auto text-xs text-slate-400 hover:text-slate-200">
          Refresh
        </button>
      </div>
      {loading && !data && <div className="text-xs text-slate-500">Loading files…</div>}
      {error && <div className="text-xs text-rose-400">Couldn’t load files: {error}</div>}
      <div className="flex flex-col divide-y divide-slate-800">
        {files.map((f) => (
          <FileRow key={f.path} id={id} file={f} onPrint={() => print(f.path)} />
        ))}
        {data && !files.length && <div className="text-xs text-slate-500">No gcode files.</div>}
      </div>
    </div>
  )
}

function FileRow({ id, file, onPrint }: { id: string; file: FileEntry; onPrint: () => void }) {
  const { data } = useMoonraker<FileMeta>(id, `/server/files/metadata?filename=${encodeURIComponent(file.path)}`)
  const m = data?.result
  const thumb = m?.thumbnails?.slice().sort((a, b) => b.width - a.width)[0]
  const name = file.path.split('/').pop() ?? file.path
  return (
    <div className="flex items-center gap-2 py-1.5">
      <div className="h-10 w-10 shrink-0 overflow-hidden rounded bg-slate-800">
        {thumb && (
          <img src={thumbUrl(id, file.path, thumb.relative_path)} alt="" className="h-full w-full object-cover" loading="lazy" />
        )}
      </div>
      <div className="min-w-0 flex-1">
        <div className="truncate text-xs text-slate-200">{name}</div>
        <div className="text-[11px] text-slate-500">
          {m?.estimated_time ? fmtDuration(m.estimated_time) : ''}
          {m?.filament_total ? ` · ${(m.filament_total / 1000).toFixed(1)} m` : ''}
        </div>
      </div>
      <button onClick={onPrint} className={btn}>
        Print
      </button>
    </div>
  )
}

function thumbUrl(id: string, filePath: string, relPath: string): string {
  const slash = filePath.lastIndexOf('/')
  const dir = slash >= 0 ? filePath.slice(0, slash + 1) : ''
  return moonrakerUrl(id, '/server/files/gcodes/' + encodeURI(dir + relPath))
}
