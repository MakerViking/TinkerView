// Package notify sends push notifications. The built-in backend is ntfy; a Log
// notifier is used when ntfy isn't configured. Adding Slack/Discord/email later =
// another Notifier in the registry.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Notification is a backend-agnostic push.
type Notification struct {
	Title    string
	Message  string
	Priority int // 1..5 (3 = default)
	Tags     []string
	Click    string   // URL opened when the notification is tapped
	Actions  []Action // e.g. a lock-screen "Pause print" button
}

// Action is an ntfy action button (http or view).
type Action struct {
	Action  string            `json:"action"` // "http" | "view"
	Label   string            `json:"label"`
	URL     string            `json:"url"`
	Method  string            `json:"method,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Clear   bool              `json:"clear,omitempty"`
}

type Notifier interface {
	Send(ctx context.Context, n Notification) error
}

// Log is the no-op/dev notifier: it just logs.
type Log struct{}

func (Log) Send(_ context.Context, n Notification) error {
	slog.Info("notify (log)", "title", n.Title, "message", n.Message, "priority", n.Priority)
	return nil
}

// Ntfy publishes to an ntfy server (POST JSON to the server root with the topic in
// the body). https://docs.ntfy.sh/publish/
type Ntfy struct {
	baseURL string
	topic   string
	token   string
	client  *http.Client
}

func NewNtfy(baseURL, topic, token string) *Ntfy {
	return &Ntfy{
		baseURL: strings.TrimRight(baseURL, "/"),
		topic:   topic,
		token:   token,
		client:  &http.Client{Timeout: 8 * time.Second},
	}
}

func (n *Ntfy) Send(ctx context.Context, notif Notification) error {
	if n.topic == "" {
		return fmt.Errorf("ntfy: no topic configured")
	}
	prio := notif.Priority
	if prio == 0 {
		prio = 3
	}
	body := map[string]any{
		"topic":    n.topic,
		"title":    notif.Title,
		"message":  notif.Message,
		"priority": prio,
	}
	if len(notif.Tags) > 0 {
		body["tags"] = notif.Tags
	}
	if notif.Click != "" {
		body["click"] = notif.Click
	}
	if len(notif.Actions) > 0 {
		body["actions"] = notif.Actions
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.baseURL, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if n.token != "" {
		req.Header.Set("Authorization", "Bearer "+n.token)
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy: HTTP %d", resp.StatusCode)
	}
	return nil
}
