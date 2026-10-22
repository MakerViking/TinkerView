import { memo, useRef, useState } from 'react'
import { useLive } from '@/store/live'
import { useWs } from '@/ws/WebSocketProvider'
import { VideoStream } from '@/components/VideoStream'
import { usePersistentBoolean } from '@/lib/persist'
import { DeviceMenu, type MenuAction } from '@/components/devices/DeviceMenu'
import { EditCameraModal } from '@/components/devices/EditCameraModal'
import { CameraSettingsModal } from '@/components/camera/CameraSettingsModal'
import { ConfirmDialog, useConfirm } from '@/components/ConfirmDialog'
import { Switch } from '@/components/ui'
import { IconCheck, IconExpand } from '@/components/icons'
import { cn, copyText } from '@/lib/utils'

// Overlay pill shared by the small in-tile actions (HQ / RTSP / HTTP).
const pillCls =
  'rounded-md bg-white/10 px-1.5 py-1 font-display text-[10px] font-semibold uppercase tracking-[0.08em] leading-none text-ink-100 backdrop-blur-sm transition-colors hover:bg-white/20'

// memo: the wall re-renders on every camera.state delta; a tile only needs to
// re-render when its own camera (selected below) or its name changes.
export const CameraTile = memo(function CameraTile({ name }: { name: string }) {
  const cam = useLive((s) => s.cameras[name])
  const { send } = useWs()
  const [copied, setCopied] = useState<'' | 'rtsp' | 'http'>('')
  const [editing, setEditing] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [settings, setSettings] = useState<'' | 'full' | 'image'>('')
  const { ask, confirmNode } = useConfirm()
  const tileRef = useRef<HTMLDivElement>(null)
  // HQ preference is remembered per camera across reloads (localStorage).
  const [hq, setHq] = usePersistentBoolean(`hq:${name}`, false)
  const streamName = hq && cam?.hasHq ? `${name}_hq` : name

  const toggle = (feature: 'detect' | 'recordings', value: boolean) =>
    send({ type: 'camera.toggle', data: { camera: name, feature, value } })

  const toggleFullscreen = () => {
    const el = tileRef.current
    if (!el) return
    if (document.fullscreenElement) void document.exitFullscreen()
    else void el.requestFullscreen?.()
  }

  // Edit… is offered for every camera (config-defined ones get a restricted form:
  // source URL only). Recording/NVR/Delete stay limited to UI-added (managed) cameras.
  const menuActions: MenuAction[] = [{ label: 'Edit…', onClick: () => setEditing(true) }]
  if (cam?.managed) {
    menuActions.push({ label: 'Recording & detection…', onClick: () => setSettings('full') })
  }
  // Image controls (rotate/flip) are available for every camera, including the
  // hand-authored config.yaml ones (those are adopted on first change).
  menuActions.push({ label: 'Image (rotate/flip)…', onClick: () => setSettings('image') })
  if (cam?.managed) {
    if (cam.promoted) {
      menuActions.push({
        label: 'Disable Frigate NVR',
        onClick: () =>
          ask({
            title: `Disable NVR for ${name}`,
            message: `Demote ${name} from Frigate? Frigate will restart (~25s). The live tile stays.`,
            confirmLabel: 'Disable NVR',
            onConfirm: () => send({ type: 'camera.demote', data: { name } }),
          }),
      })
    } else {
      menuActions.push({
        label: 'Enable Frigate NVR',
        onClick: () =>
          ask({
            title: `Enable NVR for ${name}`,
            message: `Enable recording for ${name} in Frigate? Frigate will restart (~25s).`,
            confirmLabel: 'Enable NVR',
            onConfirm: () =>
              send({
                type: 'camera.promote',
                data: { name, detect: false, record: true, recordDays: 2, snapshots: false },
              }),
          }),
      })
    }
    menuActions.push({
      label: 'Delete…',
      danger: true,
      onClick: () => setConfirmDelete(true),
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
    <div
      ref={tileRef}
      className="group relative aspect-video overflow-hidden rounded-xl border border-ink-800 bg-black shadow-elev transition-colors hover:border-ink-700"
    >
      <VideoStream src={streamName} className="h-full w-full bg-black object-contain" />

      {/* Top chrome: identity + stream actions. */}
      <div className="absolute inset-x-0 top-0 flex items-center gap-2 bg-gradient-to-b from-black/80 via-black/40 to-transparent p-2 pb-5">
        <span className="font-display text-[13px] font-semibold tracking-wide text-white drop-shadow">
          {name}
        </span>
        {cam?.promoted && (
          <span
            title="Recording in Frigate (NVR)"
            className="inline-flex items-center gap-1 rounded-md bg-rec/20 px-1.5 py-0.5 font-display text-[9px] font-bold uppercase tracking-[0.1em] text-rose-200 ring-1 ring-inset ring-rec/40"
          >
            <span className="h-1.5 w-1.5 animate-blip rounded-full bg-rec" />
            NVR
          </span>
        )}
        <div className="ml-auto flex items-center gap-1.5">
          {cam?.hasHq && (
            <button
              onClick={() => setHq((v) => !v)}
              title="Toggle the GPU-enhanced stream (denoise / sharpen / upscale)"
              aria-pressed={hq}
              className={cn(
                pillCls,
                hq && 'bg-brand-500/40 text-brand-100 ring-1 ring-inset ring-brand-400/50 hover:bg-brand-500/50',
              )}
            >
              HQ{hq ? ' ●' : ''}
            </button>
          )}
          <button
            onClick={() => copy('rtsp', rtspUrl)}
            title={`Copy RTSP URL (VLC/Frigate/ffmpeg, force TCP): ${rtspUrl}`}
            className={pillCls}
          >
            {copied === 'rtsp' ? (
              <span className="inline-flex items-center gap-1 text-emerald-300">
                <IconCheck className="text-[11px]" /> Copied
              </span>
            ) : (
              'RTSP'
            )}
          </button>
          <button
            onClick={() => copy('http', httpUrl)}
            title={`Copy HTTP player URL (browser / OBS Browser source): ${httpUrl}`}
            className={pillCls}
          >
            {copied === 'http' ? (
              <span className="inline-flex items-center gap-1 text-emerald-300">
                <IconCheck className="text-[11px]" /> Copied
              </span>
            ) : (
              'HTTP'
            )}
          </button>
          <button
            onClick={toggleFullscreen}
            title="Fullscreen"
            aria-label={`Fullscreen ${name}`}
            className="grid h-6 w-6 place-items-center rounded-md bg-white/10 text-ink-100 backdrop-blur-sm transition-colors hover:bg-white/20"
          >
            <IconExpand className="text-[13px]" />
          </button>
          {menuActions.length > 0 && <DeviceMenu actions={menuActions} />}
        </div>
      </div>

      {editing && <EditCameraModal name={name} onClose={() => setEditing(false)} />}
      {settings && (
        <CameraSettingsModal name={name} imageOnly={settings === 'image'} onClose={() => setSettings('')} />
      )}
      {confirmNode}

      {confirmDelete && (
        <ConfirmDialog
          title={`Delete camera ${name}`}
          message={
            cam?.promoted
              ? `This removes ${name} from Frigate (Frigate will restart ~25s) and from the dashboard. This cannot be undone.`
              : `This removes ${name} from the dashboard. This cannot be undone.`
          }
          confirmLabel="Delete"
          danger
          onConfirm={() => send({ type: 'camera.remove', data: { name } })}
          onClose={() => setConfirmDelete(false)}
        />
      )}

      {/* Bottom chrome: NVR feature toggles. Revealed on hover/focus; always
          visible on touch devices (no hover there). */}
      <div className="absolute inset-x-0 bottom-0 flex items-center gap-4 bg-gradient-to-t from-black/80 via-black/40 to-transparent p-2.5 pt-5 opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100 pointer-coarse:opacity-100">
        <FeatureToggle label="Detect" on={!!cam?.detect} onChange={(v) => toggle('detect', v)} />
        <FeatureToggle
          label="Record"
          on={!!cam?.recordings}
          disabled={cam ? !cam.recordConfigured : false}
          onChange={(v) => toggle('recordings', v)}
        />
      </div>
    </div>
  )
})

function FeatureToggle({
  label,
  on,
  disabled,
  onChange,
}: {
  label: string
  on: boolean
  disabled?: boolean
  onChange: (v: boolean) => void
}) {
  return (
    <span className={cn('flex items-center gap-1.5', disabled && 'opacity-50')}>
      <Switch size="sm" on={on} disabled={disabled} onChange={onChange} label={label} />
      <span className="font-display text-[10px] font-semibold uppercase tracking-[0.08em] text-ink-200 drop-shadow">
        {label}
      </span>
    </span>
  )
}
