import { useState } from 'react'
import { useLive } from '@/store/live'
import { useWs } from '@/ws/WebSocketProvider'
import { VideoStream } from '@/components/VideoStream'
import { usePersistentBoolean } from '@/lib/persist'
import { DeviceMenu, type MenuAction } from '@/components/devices/DeviceMenu'
import { AddCameraModal } from '@/components/devices/AddCameraModal'
import { cn, copyText } from '@/lib/utils'

export function CameraTile({ name }: { name: string }) {
  const cam = useLive((s) => s.cameras[name])
  const { send } = useWs()
  const [copied, setCopied] = useState<'' | 'rtsp' | 'http'>('')
  const [editing, setEditing] = useState(false)
  // HQ preference is remembered per camera across reloads (localStorage).
  const [hq, setHq] = usePersistentBoolean(`hq:${name}`, false)
  const streamName = hq && cam?.hasHq ? `${name}_hq` : name

  const toggle = (feature: 'detect' | 'recordings', value: boolean) =>
    send({ type: 'camera.toggle', data: { camera: name, feature, value } })

  // Management actions are only offered for UI-added (managed) cameras; the
  // pre-existing Frigate-config cameras stay read-only here.
  const menuActions: MenuAction[] = []
  if (cam?.managed) {
    menuActions.push({ label: 'Edit…', onClick: () => setEditing(true) })
    if (cam.promoted) {
      menuActions.push({
        label: 'Disable Frigate NVR',
        onClick: () => {
          if (confirm(`Demote ${name} from Frigate? Frigate will restart (~25s). The live tile stays.`))
            send({ type: 'camera.demote', data: { name } })
        },
      })
    } else {
      menuActions.push({
        label: 'Enable Frigate NVR',
        onClick: () => {
          if (confirm(`Enable recording for ${name} in Frigate? Frigate will restart (~25s).`))
            send({ type: 'camera.promote', data: { name, detect: false, record: true, recordDays: 2, snapshots: false } })
        },
      })
    }
    menuActions.push({
      label: 'Remove',
      danger: true,
      onClick: () => {
        if (confirm(`Remove camera ${name}?`)) send({ type: 'camera.remove', data: { name } })
      },
    })
  }

  // Two ways to consume the camera elsewhere (both via go2rtc's fan-out, so the
  // single-upstream-per-camera fix is preserved):
  //   RTSP — rtsp://host:8554/<name> for VLC/Frigate/ffmpeg (force TCP; go2rtc's
  //          RTSP server is TCP-only).
  //   HTTP — go2rtc's player page (auto WebRTC→MSE→MJPEG) for a browser or an OBS
  //          Browser source, which sidesteps the RTSP/UDP transport problem.
  const rtspUrl = `rtsp://${location.hostname}:8554/${name}`
  const httpUrl = `http://${location.hostname}:1984/stream.html?src=${name}`
  const copy = async (kind: 'rtsp' | 'http', url: string) => {
    if (await copyText(url)) {
      setCopied(kind)
      window.setTimeout(() => setCopied(''), 1500)
    }
  }

  return (
    <div className="relative rounded-lg overflow-hidden bg-black aspect-video group">
      <VideoStream src={streamName} className="w-full h-full object-contain bg-black" />

      <div className="absolute inset-x-0 top-0 flex items-center gap-2 p-2 bg-gradient-to-b from-black/70 to-transparent">
        <span className="text-sm font-medium drop-shadow">{name}</span>
        {cam?.promoted && (
          <span
            title="Recording in Frigate (NVR)"
            className="rounded bg-emerald-500/30 px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-emerald-100"
          >
            NVR
          </span>
        )}
        <div className="ml-auto flex items-center gap-2">
          {cam?.hasHq && (
            <button
              onClick={() => setHq((v) => !v)}
              title="Toggle the GPU-enhanced stream (denoise / sharpen / upscale)"
              className={cn(
                'text-[10px] uppercase tracking-wide px-1.5 py-0.5 rounded',
                hq ? 'bg-sky-500/40 text-sky-100' : 'bg-white/10 hover:bg-white/20',
              )}
            >
              {hq ? 'HQ ●' : 'HQ'}
            </button>
          )}
          <button
            onClick={() => copy('rtsp', rtspUrl)}
            title={`Copy RTSP URL (VLC/Frigate/ffmpeg, force TCP): ${rtspUrl}`}
            className="text-[10px] uppercase tracking-wide px-1.5 py-0.5 rounded bg-white/10 hover:bg-white/20"
          >
            {copied === 'rtsp' ? 'Copied ✓' : 'RTSP'}
          </button>
          <button
            onClick={() => copy('http', httpUrl)}
            title={`Copy HTTP player URL (browser / OBS Browser source): ${httpUrl}`}
            className="text-[10px] uppercase tracking-wide px-1.5 py-0.5 rounded bg-white/10 hover:bg-white/20"
          >
            {copied === 'http' ? 'Copied ✓' : 'HTTP'}
          </button>
          {menuActions.length > 0 && <DeviceMenu actions={menuActions} />}
        </div>
      </div>

      {editing && <AddCameraModal existingName={name} onClose={() => setEditing(false)} />}

      <div className="absolute inset-x-0 bottom-0 flex items-center gap-2 p-2 bg-gradient-to-t from-black/70 to-transparent opacity-0 group-hover:opacity-100 transition">
        <FeatureToggle label="Detect" on={cam?.detect} onChange={(v) => toggle('detect', v)} />
        <FeatureToggle
          label="Record"
          on={cam?.recordings}
          disabled={cam ? !cam.recordConfigured : false}
          onChange={(v) => toggle('recordings', v)}
        />
      </div>
    </div>
  )
}

function FeatureToggle({
  label,
  on,
  disabled,
  onChange,
}: {
  label: string
  on?: boolean
  disabled?: boolean
  onChange: (v: boolean) => void
}) {
  return (
    <button
      disabled={disabled}
      onClick={() => onChange(!on)}
      className={cn(
        'text-xs px-2 py-1 rounded transition',
        disabled
          ? 'bg-slate-700/40 text-slate-500 cursor-not-allowed'
          : on
            ? 'bg-emerald-500/30 text-emerald-200 hover:bg-emerald-500/40'
            : 'bg-slate-700/60 text-slate-300 hover:bg-slate-600/60',
      )}
    >
      {label}
      {on ? ' ●' : ''}
    </button>
  )
}
