package bus

import (
	"log/slog"
	"sync"
)

// Bus is a tiny in-process pub/sub. Publish never blocks on a slow subscriber:
// each subscriber has its own buffered channel, and a full channel drops the
// message for THAT subscriber only (logged). One slow consumer can therefore
// never stall a producer goroutine (an adapter). This is the load-bearing
// isolation that lets four feature areas share one process safely.
type Bus struct {
	mu   sync.RWMutex
	subs map[int]*subscription
	next int
}

type subscription struct {
	ch   chan Event
	name string
}

// New returns an empty Bus.
func New() *Bus {
	return &Bus{subs: make(map[int]*subscription)}
}

// Subscribe registers a named subscriber and returns its receive channel plus a
// cancel func that unregisters and closes it. buffer<=0 defaults to 256.
func (b *Bus) Subscribe(name string, buffer int) (<-chan Event, func()) {
	if buffer <= 0 {
		buffer = 256
	}
	b.mu.Lock()
	id := b.next
	b.next++
	sub := &subscription{ch: make(chan Event, buffer), name: name}
	b.subs[id] = sub
	b.mu.Unlock()

	return sub.ch, func() {
		b.mu.Lock()
		if s, ok := b.subs[id]; ok {
			delete(b.subs, id)
			close(s.ch)
		}
		b.mu.Unlock()
	}
}

// Publish fans an event to every subscriber without blocking. The RWMutex makes
// it safe against concurrent Subscribe/cancel (a cancel can't close a channel
// mid-Publish because both take the lock).
func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, sub := range b.subs {
		select {
		case sub.ch <- e:
		default:
			slog.Warn("bus: dropping event for slow subscriber",
				"subscriber", sub.name, "kind", e.Kind, "id", e.ID)
		}
	}
}
