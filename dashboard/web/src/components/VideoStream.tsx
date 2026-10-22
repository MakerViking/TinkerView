import { useEffect, useRef, useState } from 'react'
import { withToken } from '@/lib/auth'

// VideoStream renders a low-latency go2rtc tile. If go2rtc's <video-stream> custom
// element has been vendored (see public/README.md) it is used (WebRTC/MSE).
// Otherwise it falls back to an MJPEG <img> through the proxy — works everywhere,
// just higher latency/bandwidth. Both go through the dashboard's /go2rtc/* proxy.
// mode is go2rtc's fallback chain. The default handles BOTH source types: H.264
// cams negotiate WebRTC/MSE; MJPEG-only cams (which can't go through MSE/WebRTC)
// fall through to 'mjpeg' (served over the same WS). Order = preference.
export function VideoStream({
  src,
  mode = 'webrtc,mse,mjpeg',
  className,
}: {
  src: string
  mode?: string
  className?: string
}) {
  const ref = useRef<HTMLDivElement>(null)
  const [hasComponent, setHasComponent] = useState(
    () => typeof window !== 'undefined' && !!customElements.get('video-stream'),
  )

  // If the player script registers <video-stream> after this mounts, upgrade from
  // the MJPEG fallback to the web component (handles any script/bundle load order).
  useEffect(() => {
    if (hasComponent || typeof window === 'undefined') return
    let cancelled = false
    customElements.whenDefined('video-stream').then(() => {
      if (!cancelled) setHasComponent(true)
    })
    return () => {
      cancelled = true
    }
  }, [hasComponent])

  useEffect(() => {
    if (!hasComponent || !ref.current) return
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const el = document.createElement('video-stream') as any
    el.mode = mode
    el.media = 'video'
    el.background = true
    el.style.width = '100%'
    el.style.height = '100%'
    // withToken is a no-op unless DASHBOARD_TOKEN auth is configured.
    el.src = withToken(`${proto}://${location.host}/go2rtc/api/ws?src=${encodeURIComponent(src)}`)
    ref.current.appendChild(el)
    // oninit() has run synchronously inside connectedCallback. The vendored player
    // turns on the native control bar, whose fullscreen button only fullscreens the
    // inner <video> and breaks on live streams; the tile has its own fullscreen
    // button. Autoplay (muted, playsInline) is driven by the player, not the controls.
    if (el.video) {
      el.video.controls = false
      el.video.removeAttribute('controls')
      el.video.disablePictureInPicture = true
      el.video.setAttribute('controlsList', 'nofullscreen noremoteplayback')
    }

    // Stall watchdog. background=true makes the player skip its own visibility
    // handling and its only self-heal (onclose -> reconnect) needs the WebSocket to
    // actually close, so a silent stream with an open socket stays frozen forever.
    // If a visible, unpaused video's currentTime hasn't advanced for ~15s, reconnect.
    // Skipped for MJPEG mode (player only sets video.poster; currentTime stays 0).
    let lastTime = -1
    let stalls = 0
    let reconnectT = 0
    const watchdog = window.setInterval(() => {
      const v = el.video as HTMLVideoElement | undefined
      if (!v || document.hidden || v.paused || v.poster || el.reconnectTID) {
        stalls = 0
        return
      }
      if (Math.abs(v.currentTime - lastTime) > 0.01) {
        lastTime = v.currentTime
        stalls = 0
        return
      }
      if (++stalls < 3) return
      stalls = 0
      // Reconnect on a delay, not synchronously: the old socket's 'close' event fires
      // after ondisconnect() returns. While wsState stays CLOSED the player's onclose()
      // ignores it; reconnecting at once would let that stale event null out the new
      // socket and schedule a second connection (two sockets/PCs for one tile).
      try {
        el.ondisconnect()
        reconnectT = window.setTimeout(() => {
          reconnectT = 0
          try {
            if (el.isConnected && el.wsURL) el.onconnect()
          } catch {
            /* noop */
          }
        }, 1000)
      } catch {
        /* noop */
      }
    }, 5000)

    return () => {
      window.clearInterval(watchdog)
      window.clearTimeout(reconnectT)
      // el.src = '' alone only blanks wsURL, and el.remove() is a no-op for a
      // background player (disconnectedCallback returns early), so the WebSocket and
      // RTCPeerConnection would stay open and go2rtc would keep the consumer (and the
      // _hq NVENC session) alive. Tear down explicitly.
      try {
        if (el.reconnectTID) clearTimeout(el.reconnectTID)
        el.reconnectTID = 0
        el.wsURL = ''
        el.ondisconnect()
      } catch {
        /* noop */
      }
      el.remove()
    }
  }, [hasComponent, src, mode])

  if (!hasComponent) {
    return (
      <img
        className={className}
        src={withToken(`/go2rtc/api/stream.mjpeg?src=${encodeURIComponent(src)}`)}
        alt={src}
        loading="lazy"
      />
    )
  }
  return <div ref={ref} className={className} />
}
