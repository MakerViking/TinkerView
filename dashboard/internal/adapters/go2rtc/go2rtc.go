// Package go2rtc enumerates the cameras go2rtc is configured to serve and
// publishes a CameraState for each, so the camera wall shows real cameras even
// before the Frigate adapter exists. Detect/record stay unknown here — Frigate
// owns those toggles.
package go2rtc

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/MakerViking/TinkerView/internal/adapter"
	"github.com/MakerViking/TinkerView/internal/bus"
)

type Source struct {
	bus       *bus.Bus
	baseURL   string
	client    *http.Client
	prevNames map[string]bool // streams seen last poll (only touched from Run's goroutine)
}

func New(b *bus.Bus, baseURL string) *Source {
	return &Source{
		bus:       b,
		baseURL:   strings.TrimRight(baseURL, "/"),
		client:    &http.Client{Timeout: 5 * time.Second},
		prevNames: map[string]bool{},
	}
}

func (s *Source) Run(ctx context.Context) {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	s.poll(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.poll(ctx)
		}
	}
}

func (s *Source) poll(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/api/streams", nil)
	if err != nil {
		return
	}
	resp, err := s.client.Do(req)
	if err != nil {
		slog.Debug("go2rtc: /api/streams unreachable", "err", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	// /api/streams returns a JSON object keyed by stream name.
	var streams map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&streams); err != nil {
		return
	}
	// "<base>_hq" streams are enhanced variants of "<base>", not their own tiles.
	hqBase := map[string]bool{}
	for name := range streams {
		if b, ok := strings.CutSuffix(name, "_hq"); ok {
			hqBase[b] = true
		}
	}
	current := make(map[string]bool, len(streams))
	for name := range streams {
		if strings.HasSuffix(name, "_hq") {
			continue // surfaced as the base camera's HasHQ flag, not a separate tile
		}
		current[name] = true
		hasHQ := hqBase[name]
		s.bus.Publish(bus.Event{
			Source: bus.SourceFrigate, Kind: bus.KindCameraState, ID: name, Ts: time.Now(),
			Payload: adapter.CameraState{Name: name, HasHQ: &hasHQ},
		})
	}
	// Tell the hub to drop tiles for streams that disappeared since last poll.
	for name := range s.prevNames {
		if !current[name] {
			s.bus.Publish(bus.Event{
				Source: bus.SourceFrigate, Kind: bus.KindCameraState, ID: name, Ts: time.Now(),
				Payload: adapter.CameraState{Name: name, Deleted: true},
			})
		}
	}
	s.prevNames = current
}
