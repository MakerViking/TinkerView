import { useCallback, useEffect, useRef, useState } from 'react'
import type { ClientMsg, ServerMsg } from '@/types/ws'

export type WsStatus = 'connecting' | 'open' | 'reconnecting' | 'closed'

// A small reconnecting WebSocket: exponential backoff + jitter, app-level
// heartbeat, and NO reconnect on a clean (1000) close so a deliberate server
// restart doesn't cause a reconnect storm. On (re)open it requests a fresh
// snapshot so the store fully rehydrates.
//
// Dead-connection detection: the client pings every 25s and the server replies
// with a pong; ANY inbound frame counts as liveness. If nothing arrives for
// DEAD_AFTER_MS the socket is assumed half-open (sleep, roaming, NAT drop) and is
// replaced. 'online' and tab-visible events also trigger a prompt reconnect.
const PING_EVERY_MS = 25_000
const DEAD_AFTER_MS = 60_000
const WATCHDOG_EVERY_MS = 10_000

export function useReconnectingSocket(url: string, onMessage: (m: ServerMsg) => void) {
  const [status, setStatus] = useState<WsStatus>('connecting')
  const sockRef = useRef<WebSocket | null>(null)
  const onMessageRef = useRef(onMessage)
  onMessageRef.current = onMessage

  useEffect(() => {
    // Everything below is scoped to this effect run, so a stale socket's late
    // events (StrictMode, url change) can never touch a newer connection.
    let cancelled = false
    let timer: number | undefined
    let attempt = 0
    let lastRx = Date.now()

    const clearTimer = () => {
      if (timer !== undefined) window.clearTimeout(timer)
      timer = undefined
    }

    const scheduleReconnect = (delay: number) => {
      clearTimer()
      setStatus('reconnecting')
      timer = window.setTimeout(connect, delay)
    }

    // drop detaches and closes ws without waiting for its (possibly never-coming)
    // close event, then reconnects.
    const drop = (ws: WebSocket, delay: number) => {
      ws.onopen = ws.onmessage = ws.onclose = ws.onerror = null
      try {
        ws.close(4000, 'stale')
      } catch {
        /* already closed */
      }
      if (sockRef.current === ws) sockRef.current = null
      scheduleReconnect(delay)
    }

    function connect() {
      if (cancelled) return
      clearTimer()
      setStatus(attempt === 0 ? 'connecting' : 'reconnecting')
      const ws = new WebSocket(url)
      sockRef.current = ws
      lastRx = Date.now()

      ws.onopen = () => {
        lastRx = Date.now()
        attempt = 0
        setStatus('open')
        ws.send(JSON.stringify({ type: 'snapshot.request' }))
      }
      ws.onmessage = (ev) => {
        lastRx = Date.now()
        try {
          const m = JSON.parse(ev.data) as ServerMsg
          if (m.type === 'pong') return // liveness only
          onMessageRef.current(m)
        } catch {
          /* ignore malformed frame */
        }
      }
      ws.onclose = (ev) => {
        if (cancelled || sockRef.current !== ws) return
        sockRef.current = null
        if (ev.code === 1000) {
          setStatus('closed')
          return
        }
        const delay = Math.min(1000 * 2 ** attempt + Math.random() * 1000, 30000)
        attempt++
        scheduleReconnect(delay)
      }
      ws.onerror = () => ws.close()
    }

    // reconnectNow replaces the current socket immediately (network came back).
    const reconnectNow = () => {
      if (cancelled) return
      attempt = 0
      const ws = sockRef.current
      if (ws) drop(ws, 0)
      else scheduleReconnect(0)
    }

    const onOnline = () => reconnectNow()
    const onVisible = () => {
      if (document.visibilityState !== 'visible' || cancelled) return
      const ws = sockRef.current
      const alive = ws && ws.readyState <= WebSocket.OPEN // CONNECTING | OPEN
      if (!alive || (ws.readyState === WebSocket.OPEN && Date.now() - lastRx > DEAD_AFTER_MS)) {
        reconnectNow()
      }
    }

    connect()

    const ping = window.setInterval(() => {
      const ws = sockRef.current
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'ping', ts: Date.now() } satisfies ClientMsg))
      }
    }, PING_EVERY_MS)
    const watchdog = window.setInterval(() => {
      const ws = sockRef.current
      if (ws && ws.readyState === WebSocket.OPEN && Date.now() - lastRx > DEAD_AFTER_MS) {
        attempt = 0
        drop(ws, 0)
      }
    }, WATCHDOG_EVERY_MS)
    window.addEventListener('online', onOnline)
    document.addEventListener('visibilitychange', onVisible)

    return () => {
      cancelled = true
      clearTimer()
      window.clearInterval(ping)
      window.clearInterval(watchdog)
      window.removeEventListener('online', onOnline)
      document.removeEventListener('visibilitychange', onVisible)
      const ws = sockRef.current
      sockRef.current = null
      if (ws) {
        ws.onmessage = ws.onclose = ws.onerror = null
        // Closing a still-CONNECTING socket logs a browser warning; wait for open.
        if (ws.readyState === WebSocket.CONNECTING) ws.onopen = () => ws.close(1000, 'unmount')
        else ws.close(1000, 'unmount')
      }
    }
  }, [url])

  const send = useCallback((msg: ClientMsg) => {
    const ws = sockRef.current
    if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(msg))
  }, [])

  return { status, send }
}
