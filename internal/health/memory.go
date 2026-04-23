package health

import (
	"os"
	"path/filepath"
	"strings"
)

func collectMemory() MemoryStats {
	stats := MemoryStats{}

	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return stats
	}

	values := make(map[string]uint64)
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valStr := strings.TrimSpace(parts[1])
		valStr = strings.TrimSuffix(valStr, " kB")
		valStr = strings.TrimSpace(valStr)
		values[key] = parseUint64(valStr) * 1024 // Convert kB to bytes
	}

	stats.TotalBytes = values["MemTotal"]
	stats.AvailableBytes = values["MemAvailable"]
	if stats.TotalBytes > 0 {
		stats.UsedBytes = stats.TotalBytes - stats.AvailableBytes
		stats.UsagePercent = float64(stats.UsedBytes) * 100.0 / float64(stats.TotalBytes)
		stats.CompressedPercent = float64(zramMemUsed()) * 100.0 / float64(stats.TotalBytes)
	}

	return stats
}

// zramMemUsed returns total RAM consumed by zram compressed data across all devices.
// mm_stat fields: orig_data_size compr_data_size mem_used_total ...
// mem_used_total (field 2) is actual RAM used including metadata.
func zramMemUsed() uint64 {
	matches, _ := filepath.Glob("/sys/block/zram*/mm_stat")
	var total uint64
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		fields := strings.Fields(strings.TrimSpace(string(data)))
		if len(fields) >= 3 {
			total += parseUint64(fields[2])
		}
	}
	return total
}
