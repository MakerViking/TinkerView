package adapter

// Container is a normalized Docker container health row.
type Container struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	State     string  `json:"state"`  // running|exited|restarting|...
	Status    string  `json:"status"` // human string ("Up 3 hours")
	Health    string  `json:"health,omitempty"`
	CPUPct    float64 `json:"cpuPct"`
	MemUsed   uint64  `json:"memUsed"`
	MemLimit  uint64  `json:"memLimit"`
	UptimeSec int64   `json:"uptimeSec"`
}

// DiskUsage is a normalized filesystem usage row. Note: Free is space available
// to unprivileged users (excludes root-reserved blocks) while Used counts
// reserved blocks, so Free + Used does not equal Total — this is intentional for
// a user-facing dashboard.
type DiskUsage struct {
	Path    string  `json:"path"`
	Total   uint64  `json:"total"`
	Free    uint64  `json:"free"`
	Used    uint64  `json:"used"`
	UsedPct float64 `json:"usedPct"`
}
