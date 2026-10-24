import { useWs } from '@/ws/WebSocketProvider'
import type { WsStatus } from '@/ws/useReconnectingSocket'
import { cn } from '@/lib/utils'

// Healthy is the normal state, so it's a quiet dot; only trouble gets colour.
const MAP: Record<WsStatus, { label: string; dot: string; text: string; pulse?: boolean }> = {
  open: { label: 'Live', dot: 'bg-emerald-400', text: 'text-ink-400' },
  connecting: { label: 'Connecting', dot: 'bg-brand-400', text: 'text-brand-300', pulse: true },
  reconnecting: { label: 'Reconnecting', dot: 'bg-brand-400', text: 'text-brand-300', pulse: true },
  closed: { label: 'Offline', dot: 'bg-rec', text: 'text-rose-300' },
}

export function ConnectionStatusBadge() {
  const { status } = useWs()
  const s = MAP[status]
  return (
    <span
      title={`Dashboard link: ${status}`}
      className={cn('inline-flex items-center gap-1.5 px-1 text-[12px]', s.text)}
    >
      <span className={cn('h-1.5 w-1.5 rounded-full', s.dot, s.pulse && 'animate-blip')} />
      <span className={cn(status === 'open' && 'hidden sm:inline')}>{s.label}</span>
    </span>
  )
}
