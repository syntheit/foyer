package health

import (
	"os"
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
	}

	return stats
}
