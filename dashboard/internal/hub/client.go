package hub

import (
	"context"
	"encoding/json"
	"time"

	"github.com/coder/websocket"
)

// client is one WebSocket connection. It has a single writer goroutine draining
// `send` (coder/websocket requires one writer per conn).
type client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
	done chan struct{} // closed by removeClient; `send` itself is never closed (a send on a closed channel panics)
}

func (c *client) writeLoop(ctx context.Context) {
	for {
		select {
		case <-c.done:
			c.conn.Close(websocket.StatusNormalClosure, "")
			return
		case data := <-c.send:
			wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := c.conn.Write(wctx, websocket.MessageText, data)
			cancel()
			if err != nil {
				c.conn.Close(websocket.StatusInternalError, "write error")
				return
			}
		}
	}
}

func (c *client) readLoop(ctx context.Context) {
	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			return
		}
		var peek struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(data, &peek) != nil {
			continue
		}
		switch peek.Type {
		case "snapshot.request":
			c.enqueue(c.hub.snapshotBytes())
			continue
		case "ping":
			// Client keepalive (~25s): answer so it can detect a dead socket.
			c.enqueue(pongBytes())
			continue
		}
		c.hub.handleCommand(ctx, data)
	}
}

// enqueue queues data for the writer without blocking; it is dropped when the
// buffer is full or the client was removed. Safe against removeClient because
// c.send is never closed.
func (c *client) enqueue(data []byte) {
	select {
	case c.send <- data:
	case <-c.done:
	default:
	}
}

func pongBytes() []byte {
	data, _ := json.Marshal(ServerMsg{Type: "pong", Ts: time.Now().UnixMilli()})
	return data
}
