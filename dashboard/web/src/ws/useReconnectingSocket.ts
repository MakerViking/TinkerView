import { useCallback, useEffect, useRef, useState } from 'react'
import type { ClientMsg, ServerMsg } from '@/types/ws'

export type WsStatus = 'connecting' | 'open' | 'reconnecting' | 'closed'

// A small reconnecting WebSocket: exponential backoff + jitter, app-level
// heartbeat, and NO reconnect on a clean (1000) close so a deliberate server
// restart doesn't cause a reconnect storm. On (re)open it requests a fresh
// snapshot so the store fully rehydrates.
export function useReconnectingSocket(url: string, onMessage: (m: ServerMsg) => void) {
  const [status, setStatus] = useState<WsStatus>('connecting')
  const sockRef = useRef<WebSocket | null>(null)
  const attemptRef = useRef(0)
  const stoppedRef = useRef(false)
  const onMessageRef = useRef(onMessage)
  onMessageRef.current = onMessage

  const connect = useCallback(() => {
    if (stoppedRef.current) return
    setStatus(attemptRef.current === 0 ? 'connecting' : 'reconnecting')
    const ws = new WebSocket(url)
    sockRef.current = ws

    ws.onopen = () => {
      attemptRef.current = 0
      setStatus('open')
      ws.send(JSON.stringify({ type: 'snapshot.request' }))
    }
    ws.onmessage = (ev) => {
      try {
        onMessageRef.current(JSON.parse(ev.data) as ServerMsg)
      } catch {
        /* ignore malformed frame */
      }
    }
    ws.onclose = (ev) => {
      sockRef.current = null
      if (stoppedRef.current || ev.code === 1000) {
        setStatus('closed')
        return
      }
      const delay = Math.min(1000 * 2 ** attemptRef.current + Math.random() * 1000, 30000)
      attemptRef.current++
      setStatus('reconnecting')
      window.setTimeout(connect, delay)
    }
    ws.onerror = () => ws.close()
  }, [url])

  useEffect(() => {
    stoppedRef.current = false
    connect()
    return () => {
      stoppedRef.current = true
      sockRef.current?.close(1000, 'unmount')
    }
  }, [connect])

  const send = useCallback((msg: ClientMsg) => {
    const ws = sockRef.current
    if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(msg))
  }, [])

  useEffect(() => {
    const t = window.setInterval(() => send({ type: 'ping', ts: Date.now() }), 25000)
    return () => window.clearInterval(t)
  }, [send])

  return { status, send }
}
