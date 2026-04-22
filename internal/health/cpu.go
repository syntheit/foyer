package health

import (
	"os"
	"runtime"
	"strings"
)

type cpuTicks struct {
	user, nice, system, idle, iowait, irq, softirq uint64
}

func (t *cpuTicks) total() uint64 {
	return t.user + t.nice + t.system + t.idle + t.iowait + t.irq + t.softirq
}

func (t *cpuTicks) busy() uint64 {
	return t.user + t.nice + t.system + t.irq + t.softirq
}

func collectCPU(prev *cpuTicks) (CPUStats, *cpuTicks) {
	stats := CPUStats{
		Cores: runtime.NumCPU(),
	}

	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return stats, prev
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return stats, prev
	}

	fields := strings.Fields(lines[0])
	if len(fields) < 8 || fields[0] != "cpu" {
		return stats, prev
	}

	now := &cpuTicks{
		user:    parseUint64(fields[1]),
		nice:    parseUint64(fields[2]),
		system:  parseUint64(fields[3]),
		idle:    parseUint64(fields[4]),
		iowait:  parseUint64(fields[5]),
		irq:     parseUint64(fields[6]),
		softirq: parseUint64(fields[7]),
	}

	if prev != nil {
		dTotal := now.total() - prev.total()
		dBusy := now.busy() - prev.busy()
		if dTotal > 0 {
			stats.UsagePercent = float64(dBusy) * 100.0 / float64(dTotal)
		}
	}

	// Load is set by the collector to avoid double-reading
	return stats, now
}

func readLoadAvg() []float64 {
	data, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return nil
	}
	fields := strings.Fields(string(data))
	if len(fields) < 3 {
		return nil
	}
	return []float64{
		parseFloat64(fields[0]),
		parseFloat64(fields[1]),
		parseFloat64(fields[2]),
	}
}
