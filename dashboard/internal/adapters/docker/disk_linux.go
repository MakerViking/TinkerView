//go:build linux

package docker

import (
	"log/slog"
	"syscall"
	"time"

	"github.com/MakerViking/TinkerView/internal/adapter"
	"github.com/MakerViking/TinkerView/internal/bus"
)

func (s *Source) pollDisks() {
	var out []adapter.DiskUsage
	for _, path := range s.diskPaths {
		var fs syscall.Statfs_t
		if err := syscall.Statfs(path, &fs); err != nil {
			slog.Debug("docker: statfs failed", "path", path, "err", err)
			continue
		}
		bs := uint64(fs.Bsize)
		total := fs.Blocks * bs
		free := fs.Bavail * bs // available to unprivileged users
		used := (fs.Blocks - fs.Bfree) * bs
		var pct float64
		if total > 0 {
			pct = float64(used) / float64(total) * 100
		}
		out = append(out, adapter.DiskUsage{Path: path, Total: total, Free: free, Used: used, UsedPct: pct})
	}
	// Always publish (even when empty) so a total statfs failure clears the panel
	// rather than leaving stale data, matching the container poll behaviour.
	s.bus.Publish(bus.Event{Source: bus.SourceDocker, Kind: bus.KindDisks, Ts: time.Now(), Payload: out})
}
