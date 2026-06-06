package go2rtc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a runtime control client for go2rtc's streams API (distinct from the
// read-only Source, which only enumerates). The dashboard uses it to add/remove
// streams for UI-managed cameras. Runtime changes are not persisted by go2rtc
// (Frigate generates its config), so the CameraManager reconciles them.
type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 8 * time.Second},
	}
}

// ListStreams returns the current streams keyed by name.
func (c *Client) ListStreams(ctx context.Context) (map[string]json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/streams", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("go2rtc list streams: HTTP %d", resp.StatusCode)
	}
	var streams map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&streams); err != nil {
		return nil, err
	}
	return streams, nil
}

// PutStream creates or replaces the stream named name with source src.
func (c *Client) PutStream(ctx context.Context, name, src string) error {
	q := url.Values{"name": {name}, "src": {src}}
	return c.do(ctx, http.MethodPut, q)
}

// DeleteStream removes the stream named name. go2rtc's DELETE selects the stream
// by the (confusingly named) src query parameter, which holds the stream name.
func (c *Client) DeleteStream(ctx context.Context, name string) error {
	q := url.Values{"src": {name}}
	return c.do(ctx, http.MethodDelete, q)
}

func (c *Client) do(ctx context.Context, method string, q url.Values) error {
	u := c.baseURL + "/api/streams?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("go2rtc %s streams: %w", method, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("go2rtc %s streams: HTTP %d %s", method, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
