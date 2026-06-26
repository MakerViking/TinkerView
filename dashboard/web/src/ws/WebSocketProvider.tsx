import { createContext, useContext, useEffect, type ReactNode } from 'react'
import { useReconnectingSocket, type WsStatus } from './useReconnectingSocket'
import { useLive } from '@/store/live'
import { withToken } from '@/lib/auth'
import type { ClientMsg } from '@/types/ws'

interface WsContext {
  status: WsStatus
  send: (m: ClientMsg) => void
}

const Ctx = createContext<WsContext>({ status: 'connecting', send: () => {} })

export const useWs = () => useContext(Ctx)

function wsUrl(): string {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  // ?token= when a shared token is configured (the WS can't send an auth header).
  return withToken(`${proto}://${location.host}/ws`)
}

export function WebSocketProvider({ children }: { children: ReactNode }) {
  const apply = useLive((s) => s.applyServerMsg)
  const setConnected = useLive((s) => s.setConnected)
  const { status, send } = useReconnectingSocket(wsUrl(), apply)

  useEffect(() => {
    setConnected(status === 'open')
  }, [status, setConnected])

  return <Ctx.Provider value={{ status, send }}>{children}</Ctx.Provider>
}
