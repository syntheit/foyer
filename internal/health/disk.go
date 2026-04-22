package health

import (
	"os/exec"
	"strings"
	"syscall"
)

func collectDisk() DiskStats {
	stats := DiskStats{}
	stats.Pools = collectZFSPools()
	stats.Mounts = collectMounts()
	return stats
}

func collectZFSPools() []PoolStats {
	// Use explicit column selection to avoid format differences between ZFS versions
	out, err := exec.Command("zpool", "list", "-Hpo", "name,size,alloc,free,health").Output()
	if err != nil {
		return nil
	}

	var pools []PoolStats
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		total := parseUint64(fields[1])
		used := parseUint64(fields[2])
		free := parseUint64(fields[3])
		health := fields[4]

		var usage float64
		if total > 0 {
			usage = float64(used) * 100.0 / float64(total)
		}

		pools = append(pools, PoolStats{
			Name:         fields[0],
			TotalBytes:   total,
			UsedBytes:    used,
			FreeBytes:    free,
			UsagePercent: usage,
			Health:       health,
		})
	}
	return pools
}

func collectMounts() []MountStats {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err != nil {
		return nil
	}

	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	used := total - free
	var usage float64
	if total > 0 {
		usage = float64(used) * 100.0 / float64(total)
	}

	return []MountStats{{
		Mountpoint:   "/",
		Filesystem:   "root",
		TotalBytes:   total,
		UsedBytes:    used,
		UsagePercent: usage,
	}}
}
