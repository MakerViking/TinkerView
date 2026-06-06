//go:build windows

package docker

import (
	"syscall"
	"time"
	"unsafe"

	"github.com/MakerViking/TinkerView/internal/adapter"
	"github.com/MakerViking/TinkerView/internal/bus"
)

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procGetDiskFreeSpaceEx = kernel32.NewProc("GetDiskFreeSpaceExW")
)

func (s *Source) pollDisks() {
	var out []adapter.DiskUsage
	for _, path := range s.diskPaths {
		p, err := syscall.UTF16PtrFromString(path)
		if err != nil {
			continue
		}
		var freeAvail, total, totalFree uint64
		r, _, _ := procGetDiskFreeSpaceEx.Call(
			uintptr(unsafe.Pointer(p)),
			uintptr(unsafe.Pointer(&freeAvail)),
			uintptr(unsafe.Pointer(&total)),
			uintptr(unsafe.Pointer(&totalFree)),
		)
		if r == 0 {
			continue // call failed
		}
		used := total - totalFree
		var pct float64
		if total > 0 {
			pct = float64(used) / float64(total) * 100
		}
		out = append(out, adapter.DiskUsage{Path: path, Total: total, Free: freeAvail, Used: used, UsedPct: pct})
	}
	// Always publish (even when empty) so a total failure clears the panel rather
	// than leaving stale data, matching the container poll behaviour.
	s.bus.Publish(bus.Event{Source: bus.SourceDocker, Kind: bus.KindDisks, Ts: time.Now(), Payload: out})
}
