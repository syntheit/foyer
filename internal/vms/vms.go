// Package vms reports libvirt VM aggregate stats for the host dashboard tile.
//
// Foyer itself has no libvirt access; we ask foyer-vm-controller over its Unix
// socket. If the socket isn't configured (e.g. libvirtd disabled, dev mode),
// Collect returns nil and the dashboard hides the tile.
package vms

import (
	"strconv"

	"github.com/dmiller/foyer/internal/vmcontrol"
)

type Stats struct {
	Total      int      `json:"total"`
	Running    int      `json:"running"`
	TotalMemMB uint64   `json:"total_memory_mb"`
	TotalVCPUs int      `json:"total_vcpus"`
	VMs        []VMInfo `json:"vms"`
}

type VMInfo struct {
	Name     string `json:"name"`
	State    string `json:"state"`
	MemoryMB uint64 `json:"memory_mb"`
	VCPUs    int    `json:"vcpus"`
}

// Collect returns nil when no controller socket is configured. Errors talking
// to the controller also yield nil — the dashboard tile is best-effort.
func Collect(socketPath string) *Stats {
	if socketPath == "" {
		return nil
	}
	resp, err := vmcontrol.NewClient(socketPath).Call(vmcontrol.ActionListInfo, "")
	if err != nil || resp == nil {
		return nil
	}
	rows, ok := resp.Data.([]interface{})
	if !ok {
		return nil
	}

	stats := &Stats{VMs: make([]VMInfo, 0, len(rows))}
	for _, r := range rows {
		m, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		info := VMInfo{
			Name:  asString(m["name"]),
			State: asString(m["state"]),
			VCPUs: int(asUint64(m["vcpus"])),
		}
		// Controller reports KiB; the existing dashboard tile expects MB.
		info.MemoryMB = asUint64(m["mem_max_kib"]) / 1024

		stats.VMs = append(stats.VMs, info)
		stats.Total++
		if info.State == "running" {
			stats.Running++
			stats.TotalMemMB += info.MemoryMB
			stats.TotalVCPUs += info.VCPUs
		}
	}
	return stats
}

func asString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func asUint64(v interface{}) uint64 {
	switch n := v.(type) {
	case float64:
		return uint64(n)
	case string:
		u, _ := strconv.ParseUint(n, 10, 64)
		return u
	}
	return 0
}
