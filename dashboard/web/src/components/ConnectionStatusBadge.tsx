import { useWs } from '@/ws/WebSocketProvider'
import type { WsStatus } from '@/ws/useReconnectingSocket'
import { Badge, type BadgeTone } from '@/components/ui'

const MAP: Record<WsStatus, { label: string; tone: BadgeTone; pulse?: boolean }> = {
  open: { label: 'Live', tone: 'ok', pulse: true },
  connecting: { label: 'Connecting', tone: 'warn', pulse: true },
  reconnecting: { label: 'Reconnecting', tone: 'warn', pulse: true },
  closed: { label: 'Offline', tone: 'danger' },
}

export function ConnectionStatusBadge() {
  const { status } = useWs()
  const s = MAP[status]
  return (
    <Badge tone={s.tone} dot pulse={s.pulse} title={`Dashboard link: ${status}`}>
      {s.label}
    </Badge>
  )
}
