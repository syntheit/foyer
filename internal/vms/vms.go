// Package vms reports libvirt VM stats. Shells out to `virsh` so we don't
// take a libvirt-go dep — VM state is small and updates infrequently, so the
// cost of fork+exec is acceptable on the collector's tick.
package vms

import (
	"bufio"
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
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

const uri = "qemu:///system"

// Collect returns nil if virsh is unavailable or libvirtd isn't reachable.
func Collect() *Stats {
	if _, err := exec.LookPath("virsh"); err != nil {
		return nil
	}

	names, err := listDomains()
	if err != nil {
		return nil
	}

	stats := &Stats{VMs: make([]VMInfo, 0, len(names))}
	for _, name := range names {
		info := domInfo(name)
		if info == nil {
			continue
		}
		stats.VMs = append(stats.VMs, *info)
		stats.Total++
		if info.State == "running" {
			stats.Running++
			stats.TotalMemMB += info.MemoryMB
			stats.TotalVCPUs += info.VCPUs
		}
	}
	return stats
}

func listDomains() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "virsh", "-c", uri, "list", "--all", "--name").Output()
	if err != nil {
		return nil, err
	}
	var names []string
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		n := strings.TrimSpace(sc.Text())
		if n != "" {
			names = append(names, n)
		}
	}
	return names, nil
}

func domInfo(name string) *VMInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "virsh", "-c", uri, "dominfo", name).Output()
	if err != nil {
		return nil
	}

	info := &VMInfo{Name: name, State: "unknown"}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "State":
			info.State = v
		case "CPU(s)":
			info.VCPUs, _ = strconv.Atoi(v)
		case "Used memory":
			// "1048576 KiB"
			fields := strings.Fields(v)
			if len(fields) > 0 {
				kib, _ := strconv.ParseUint(fields[0], 10, 64)
				info.MemoryMB = kib / 1024
			}
		}
	}
	return info
}
