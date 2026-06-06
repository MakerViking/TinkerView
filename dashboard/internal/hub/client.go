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
	done chan struct{} // closed by removeClient; guards sends against a closed `send`
}

func (c *client) writeLoop(ctx context.Context) {
	for data := range c.send {
		wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := c.conn.Write(wctx, websocket.MessageText, data)
		cancel()
		if err != nil {
			c.conn.Close(websocket.StatusInternalError, "write error")
			return
		}
	}
	c.conn.Close(websocket.StatusNormalClosure, "")
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
		if peek.Type == "snapshot.request" {
			// Guard against removeClient having closed c.send concurrently (a send
			// on a closed channel panics; the default case does not prevent that).
			select {
			case c.send <- c.hub.snapshotBytes():
			case <-c.done:
				return
			default:
			}
			continue
		}
		c.hub.handleCommand(ctx, data)
	}
}
