package hub

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MakerViking/TinkerView/internal/bus"
	"github.com/coder/websocket"
)

func newTestServer(t *testing.T, token string, origins []string) (*Hub, string) {
	t.Helper()
	h := New(bus.New(), nil, token)
	h.SetAllowedOrigins(origins)
	srv := httptest.NewServer(http.HandlerFunc(h.ServeWS))
	t.Cleanup(srv.Close)
	return h, "ws" + strings.TrimPrefix(srv.URL, "http")
}

func readType(t *testing.T, ctx context.Context, c *websocket.Conn) string {
	t.Helper()
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	s := string(data)
	switch {
	case strings.Contains(s, `"type":"snapshot"`):
		return "snapshot"
	case strings.Contains(s, `"type":"pong"`):
		return "pong"
	}
	return s
}

func TestPingGetsPong(t *testing.T) {
	_, url := newTestServer(t, "", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	if got := readType(t, ctx, c); got != "snapshot" {
		t.Fatalf("first message = %s, want snapshot", got)
	}
	if err := c.Write(ctx, websocket.MessageText, []byte(`{"type":"ping","ts":1}`)); err != nil {
		t.Fatal(err)
	}
	if got := readType(t, ctx, c); got != "pong" {
		t.Fatalf("reply = %s, want pong", got)
	}
}

func TestOriginCheck(t *testing.T) {
	_, url := newTestServer(t, "", []string{"dev.example:5173"})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dial := func(origin string) error {
		c, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {origin}}})
		if err == nil {
			c.CloseNow()
		}
		return err
	}
	if err := dial("http://evil.example"); err == nil {
		t.Fatal("cross-site origin must be rejected")
	}
	if err := dial("http://dev.example:5173"); err != nil {
		t.Fatalf("allow-listed origin rejected: %v", err)
	}
	// Same-host origin (what the SPA sends) is accepted.
	host := strings.TrimPrefix(url, "ws://")
	if err := dial("http://" + host); err != nil {
		t.Fatalf("same-host origin rejected: %v", err)
	}
}

func TestTokenRequired(t *testing.T) {
	_, url := newTestServer(t, "s3cret", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, _, err := websocket.Dial(ctx, url+"?token=wrong", nil); err == nil {
		t.Fatal("wrong token must be rejected")
	}
	c, _, err := websocket.Dial(ctx, url+"?token=s3cret", nil)
	if err != nil {
		t.Fatalf("right token rejected: %v", err)
	}
	c.CloseNow()
}

// A slow client is dropped by broadcast while its readLoop keeps handling
// requests; neither side may panic on a send to a closed channel.
func TestRemoveClientConcurrentWithEnqueue(t *testing.T) {
	h := New(bus.New(), nil, "")
	c := &client{hub: h, send: make(chan []byte, 1), done: make(chan struct{})}
	h.clients[c] = struct{}{}
	go func() {
		for i := 0; i < 1000; i++ {
			c.enqueue([]byte("x"))
		}
	}()
	h.removeClient(c)
	h.removeClient(c) // idempotent
	for i := 0; i < 1000; i++ {
		c.enqueue([]byte("x"))
	}
}
