import { useEffect, useRef, useState } from 'react'

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
    el.src = `${proto}://${location.host}/go2rtc/api/ws?src=${encodeURIComponent(src)}`
    ref.current.appendChild(el)
    return () => {
      try {
        el.src = ''
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
        src={`/go2rtc/api/stream.mjpeg?src=${encodeURIComponent(src)}`}
        alt={src}
        loading="lazy"
      />
    )
  }
  return <div ref={ref} className={className} />
}
