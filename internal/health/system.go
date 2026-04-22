package health

import (
	"os"
	"strings"
)

func collectSystem() SystemStats {
	stats := SystemStats{}

	if data, err := os.ReadFile("/etc/hostname"); err == nil {
		stats.Hostname = strings.TrimSpace(string(data))
	}

	if data, err := os.ReadFile("/proc/version"); err == nil {
		parts := strings.Fields(string(data))
		if len(parts) >= 3 {
			stats.Kernel = parts[2]
		}
	}

	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 1 {
			stats.UptimeSeconds = uint64(parseFloat64(fields[0]))
		}
	}

	// LoadAvg is set by the collector to avoid double-reading
	return stats
}
