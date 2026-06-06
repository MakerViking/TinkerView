// Package docker surfaces container health (state, CPU%, memory, uptime) and disk
// usage on the bus, and performs allow-listed container restarts. It speaks the
// Docker Engine REST API directly over HTTP — works against the tecnativa
// docker-socket-proxy (tcp://) or the raw/podman socket (unix://) — so it needs
// none of the heavy docker/docker client dependency tree. Only the
// list/stats/restart endpoints are ever called.
package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/MakerViking/TinkerView/internal/adapter"
	"github.com/MakerViking/TinkerView/internal/bus"
)

type Source struct {
	bus       *bus.Bus
	base      string // e.g. http://127.0.0.1:2375  or  http://unix
	client    *http.Client
	watch     map[string]bool // allow-list of container names; empty = all
	diskPaths []string
}

// New builds the Docker source. host is a Docker endpoint: tcp://host:2375,
// http://host:2375, or unix:///var/run/docker.sock.
func New(b *bus.Bus, host string, watch, diskPaths []string) (*Source, error) {
	base, transport, err := dialFor(host)
	if err != nil {
		return nil, err
	}
	w := map[string]bool{}
	for _, n := range watch {
		w[n] = true
	}
	return &Source{
		bus:  b,
		base: base,
		// No blanket timeout: a restart legitimately takes up to the stop grace
		// period (~10s). Per-request deadlines are applied below instead.
		client: &http.Client{Transport: transport},
		watch:  w, diskPaths: diskPaths,
	}, nil
}

// dialFor returns the HTTP base URL and a transport for the given Docker host.
func dialFor(host string) (string, *http.Transport, error) {
	switch {
	case strings.HasPrefix(host, "unix://"):
		sock := strings.TrimPrefix(host, "unix://")
		return "http://unix", &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", sock)
			},
		}, nil
	case host == "":
		return "", nil, fmt.Errorf("docker: empty host")
	default:
		h := strings.Replace(host, "tcp://", "http://", 1)
		if !strings.HasPrefix(h, "http://") && !strings.HasPrefix(h, "https://") {
			h = "http://" + h
		}
		return strings.TrimRight(h, "/"), &http.Transport{}, nil
	}
}

func (s *Source) Run(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
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
	s.pollContainers(ctx)
	s.pollDisks()
}

type apiContainer struct {
	ID      string   `json:"Id"`
	Names   []string `json:"Names"`
	State   string   `json:"State"`
	Status  string   `json:"Status"`
	Created int64    `json:"Created"`
}

type dockerStats struct {
	CPUStats struct {
		CPUUsage struct {
			TotalUsage  uint64   `json:"total_usage"`
			PercpuUsage []uint64 `json:"percpu_usage"`
		} `json:"cpu_usage"`
		SystemUsage uint64 `json:"system_cpu_usage"`
		OnlineCPUs  uint32 `json:"online_cpus"`
	} `json:"cpu_stats"`
	PreCPUStats struct {
		CPUUsage struct {
			TotalUsage uint64 `json:"total_usage"`
		} `json:"cpu_usage"`
		SystemUsage uint64 `json:"system_cpu_usage"`
	} `json:"precpu_stats"`
	MemoryStats struct {
		Usage uint64 `json:"usage"`
		Limit uint64 `json:"limit"`
	} `json:"memory_stats"`
}

func (s *Source) pollContainers(ctx context.Context) {
	var list []apiContainer
	if err := s.getJSON(ctx, "/containers/json?all=true", &list); err != nil {
		slog.Warn("docker: container list failed", "err", err)
		return
	}
	now := time.Now().Unix()
	out := make([]adapter.Container, 0, len(list))
	for _, c := range list {
		name := strings.TrimPrefix(firstName(c.Names), "/")
		if len(s.watch) > 0 && !s.watch[name] {
			continue
		}
		row := adapter.Container{ID: c.ID, Name: name, State: c.State, Status: c.Status}
		if c.State == "running" {
			if c.Created > 0 {
				row.UptimeSec = now - c.Created
			}
			s.addStats(ctx, c.ID, &row)
		}
		out = append(out, row)
	}
	s.bus.Publish(bus.Event{Source: bus.SourceDocker, Kind: bus.KindContainers, Ts: time.Now(), Payload: out})
}

func (s *Source) addStats(ctx context.Context, id string, row *adapter.Container) {
	var st dockerStats
	// stream=false returns one snapshot WITH precpu_stats populated, so CPU% is
	// computable from a single request (no need to keep cross-poll samples).
	if err := s.getJSON(ctx, "/containers/"+id+"/stats?stream=false", &st); err != nil {
		return
	}
	row.MemUsed = st.MemoryStats.Usage
	row.MemLimit = st.MemoryStats.Limit
	// int64 deltas so a stale precpu > cpu shows up as negative (and is skipped)
	// rather than wrapping to a giant uint64 → bogus 1000%+ readings.
	cpuDelta := int64(st.CPUStats.CPUUsage.TotalUsage) - int64(st.PreCPUStats.CPUUsage.TotalUsage)
	sysDelta := int64(st.CPUStats.SystemUsage) - int64(st.PreCPUStats.SystemUsage)
	ncpu := float64(st.CPUStats.OnlineCPUs)
	if ncpu == 0 {
		ncpu = float64(len(st.CPUStats.CPUUsage.PercpuUsage))
	}
	if ncpu == 0 {
		ncpu = 1
	}
	if sysDelta > 0 && cpuDelta > 0 {
		row.CPUPct = float64(cpuDelta) / float64(sysDelta) * ncpu * 100
	}
}

// Restart restarts a container, but only if it is in the allow-list. It resolves
// the input to a canonical container first and validates the RESOLVED name, then
// acts on the RESOLVED id — never the raw input — so the check and the action
// always refer to the same container.
func (s *Source) Restart(ctx context.Context, idOrName string) error {
	id, name, err := s.resolve(ctx, idOrName)
	if err != nil {
		return err
	}
	if len(s.watch) > 0 && !s.watch[name] {
		return fmt.Errorf("container %q is not in the restart allow-list", name)
	}
	rctx, cancel := context.WithTimeout(ctx, 25*time.Second) // allow for the stop grace period
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodPost, s.base+"/containers/"+id+"/restart?t=10", nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("restart %s: HTTP %d", name, resp.StatusCode)
	}
	slog.Info("docker: restarted container", "name", name)
	return nil
}

// resolve looks up a container fresh (not the poll cache) by full id, id-prefix,
// or name, returning its canonical id and name.
func (s *Source) resolve(ctx context.Context, idOrName string) (id, name string, err error) {
	var list []apiContainer
	if err := s.getJSON(ctx, "/containers/json?all=true", &list); err != nil {
		return "", "", err
	}
	want := strings.TrimPrefix(idOrName, "/")
	for _, c := range list {
		cname := strings.TrimPrefix(firstName(c.Names), "/")
		if c.ID == want || cname == want || (len(want) >= 12 && strings.HasPrefix(c.ID, want)) {
			return c.ID, cname, nil
		}
	}
	return "", "", fmt.Errorf("container %q not found", idOrName)
}

func (s *Source) getJSON(ctx context.Context, path string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.base+path, nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("GET %s: HTTP %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func firstName(names []string) string {
	if len(names) > 0 {
		return names[0]
	}
	return ""
}
