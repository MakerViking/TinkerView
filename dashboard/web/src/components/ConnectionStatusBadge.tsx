import { useWs } from '@/ws/WebSocketProvider'
import type { WsStatus } from '@/ws/useReconnectingSocket'
import { cn } from '@/lib/utils'

const MAP: Record<WsStatus, { label: string; cls: string }> = {
  open: { label: '● Live', cls: 'bg-emerald-500/20 text-emerald-300' },
  connecting: { label: 'Connecting…', cls: 'bg-amber-500/20 text-amber-300' },
  reconnecting: { label: 'Reconnecting…', cls: 'bg-amber-500/20 text-amber-300' },
  closed: { label: 'Offline', cls: 'bg-rose-500/20 text-rose-300' },
}

export function ConnectionStatusBadge() {
  const { status } = useWs()
  const s = MAP[status]
  return <span className={cn('px-2 py-1 rounded-md text-xs font-medium', s.cls)}>{s.label}</span>
}
