import type { CameraSrcKind } from '@/types/ws'

export const SRC_KINDS: { value: CameraSrcKind; label: string; hint: string }[] = [
  { value: 'rtsp', label: 'RTSP (H.264)', hint: 'rtsp://host:554/stream — passthrough; put the login in the fields below' },
  { value: 'mjpeg', label: 'MJPEG printer cam', hint: 'http://host/webcam/?action=stream — transcoded to H.264 for live view' },
  { value: 'h264', label: 'Raw H.264 over HTTP', hint: 'http://host/webcam/stream.h264 (e.g. Snapmaker)' },
  { value: 'mjpeg_raw', label: 'MJPEG (no transcode)', hint: 'Low CPU; falls back to MJPEG/MSE, cannot WebRTC' },
  { value: 'advanced', label: 'Advanced (raw go2rtc source)', hint: 'Power user: full go2rtc source string' },
]

// Mirrors the server's camera-name rule (lowercase letters, digits, _; the _hq
// suffix is reserved for the GPU-enhanced twin stream). The server stays authoritative.
export function validCameraName(n: string): boolean {
  return /^[a-z0-9_]+$/.test(n) && !n.endsWith('_hq')
}
