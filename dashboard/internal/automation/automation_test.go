package automation

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/MakerViking/TinkerView/internal/adapter"
	"github.com/MakerViking/TinkerView/internal/bus"
	"github.com/MakerViking/TinkerView/internal/notify"
)

type countNotifier struct {
	mu sync.Mutex
	n  int
}

func (c *countNotifier) Send(context.Context, notify.Notification) error {
	c.mu.Lock()
	c.n++
	c.mu.Unlock()
	return nil
}

func TestOfflineBlipDoesNotRefire(t *testing.T) {
	cn := &countNotifier{}
	e := New(bus.New(), cn, nil)
	send := func(s adapter.PState) {
		e.handle(context.Background(), bus.Event{Kind: bus.KindPrinterState, Payload: adapter.PrinterState{ID: "p", Name: "P", State: s}})
	}
	send(adapter.Printing)
	send(adapter.Complete) // fires once
	if cn.n != 1 {
		t.Fatalf("after completion: %d notifications, want 1", cn.n)
	}
	// Wi-Fi blip longer than the cooldown must not re-announce the same print.
	e.lastFired = map[string]time.Time{}
	send(adapter.Offline)
	send(adapter.Complete)
	if cn.n != 1 {
		t.Fatalf("offline blip re-fired: %d notifications, want 1", cn.n)
	}
}
