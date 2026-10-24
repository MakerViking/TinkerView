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

// Square-ish overlay button shared by the in-tile controls (HQ / fullscreen).
const ctrlCls =
  'grid h-7 min-w-7 place-items-center rounded-md bg-black/45 px-1.5 text-[11px] font-semibold text-ink-100 transition-colors hover:bg-black/70'

// memo: the wall re-renders on every camera.state delta; a tile only needs to
// re-render when its own camera (selected below) or its name changes.
export const CameraTile = memo(function CameraTile({ name }: { name: string }) {
  const cam = useLive((s) => s.cameras[name])
  const { send } = useWs()
  const [copied, setCopied] = useState('')
  const [editing, setEditing] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState(false)
  const [settings, setSettings] = useState<'' | 'full' | 'image'>('')
  const { ask, confirmNode } = useConfirm()
  const tileRef = useRef<HTMLDivElement>(null)
  // HQ preference is remembered per camera across reloads (localStorage).
  const [hq, setHq] = usePersistentBoolean(`hq:${name}`, false)
  const streamName = hq && cam?.hasHq ? `${name}_hq` : name

  // Two ways to consume the camera elsewhere (both via go2rtc's fan-out, so the
  // single-upstream-per-camera fix is preserved):
  //   RTSP — rtsp://host:8554/<name> for VLC/Frigate/ffmpeg (force TCP; go2rtc's
  //          RTSP server is TCP-only).
  //   HTTP — go2rtc's player page (auto WebRTC→MSE→MJPEG) for a browser or an OBS
  //          Browser source, which sidesteps the RTSP/UDP transport problem.
  const rtspUrl = `rtsp://${location.hostname}:8554/${name}`
  const httpUrl = `http://${location.hostname}:1984/stream.html?src=${name}`
  const copy = async (what: string, url: string) => {
    if (await copyText(url)) {
      setCopied(what)
      window.setTimeout(() => setCopied(''), 1600)
    }
  }

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
  const menuActions: MenuAction[] = [
    { label: 'Copy RTSP URL', onClick: () => copy('RTSP URL', rtspUrl) },
    { label: 'Copy player URL', onClick: () => copy('Player URL', httpUrl) },
    { label: 'Edit…', separator: true, onClick: () => setEditing(true) },
  ]
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
      separator: true,
      onClick: () => setConfirmDelete(true),
    })
  }

  return (
    <div
      ref={tileRef}
      className="group relative aspect-video overflow-hidden rounded-lg bg-ink-1000 ring-1 ring-white/[0.06] transition-shadow hover:ring-white/[0.12]"
    >
      <VideoStream src={streamName} className="h-full w-full bg-ink-1000 object-contain" />

      {/* Top-right controls: out of the picture until hovered/focused; always
          shown on touch devices (no hover there). */}
      <div className="absolute right-2 top-2 flex items-center gap-1 opacity-0 transition-opacity duration-150 group-hover:opacity-100 group-focus-within:opacity-100 pointer-coarse:opacity-100">
        {cam?.hasHq && (
          <button
            onClick={() => setHq((v) => !v)}
            title="GPU-enhanced stream (denoise / sharpen)"
            aria-pressed={hq}
            className={cn(ctrlCls, hq && 'bg-brand-400 text-ink-1000 hover:bg-brand-300')}
          >
            HQ
          </button>
        )}
        <button onClick={toggleFullscreen} title="Fullscreen" aria-label={`Fullscreen ${name}`} className={ctrlCls}>
          <IconExpand className="text-[13px]" />
        </button>
        {menuActions.length > 0 && <DeviceMenu actions={menuActions} />}
      </div>

      {/* Bottom scrim: identity on the left, NVR toggles on hover at the right. */}
      <div className="pointer-events-none absolute inset-x-0 bottom-0 flex items-end gap-3 bg-gradient-to-t from-black/75 via-black/25 to-transparent px-3 pb-2.5 pt-8">
        <div className="flex min-w-0 items-center gap-2">
          <span className="truncate text-[13px] font-medium text-white">{name}</span>
          {cam?.promoted && (
            <span title="Recording in Frigate (NVR)" className="flex items-center gap-1 text-[11px] text-rose-200/90">
              <span className="h-1.5 w-1.5 rounded-full bg-rec" />
              Rec
            </span>
          )}
          {hq && cam?.hasHq && <span className="text-[11px] font-medium text-brand-300">HQ</span>}
          {copied && (
            <span className="flex items-center gap-1 text-[11px] text-emerald-300 animate-fade">
              <IconCheck className="text-[11px]" /> {copied} copied
            </span>
          )}
        </div>
        <div className="pointer-events-auto ml-auto flex items-center gap-3 opacity-0 transition-opacity duration-150 group-hover:opacity-100 group-focus-within:opacity-100 pointer-coarse:opacity-100">
          <FeatureToggle label="Detect" on={!!cam?.detect} onChange={(v) => toggle('detect', v)} />
          <FeatureToggle
            label="Record"
            on={!!cam?.recordings}
            disabled={cam ? !cam.recordConfigured : false}
            onChange={(v) => toggle('recordings', v)}
          />
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
      <span className="text-[11px] text-ink-200">{label}</span>
    </span>
  )
}
