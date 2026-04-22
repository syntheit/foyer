package health

import (
	"os"
	"strings"
	"time"
)

type netCounters struct {
	rxBytes uint64
	txBytes uint64
	at      time.Time
}

func collectNetwork(prev map[string]netCounters) (NetworkStats, map[string]netCounters) {
	stats := NetworkStats{}
	current := make(map[string]netCounters)
	now := time.Now()

	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return stats, current
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines[2:] { // Skip header lines
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		colonIdx := strings.Index(line, ":")
		if colonIdx < 0 {
			continue
		}

		name := strings.TrimSpace(line[:colonIdx])
		// Skip loopback and virtual interfaces
		if name == "lo" || strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "br-") || strings.HasPrefix(name, "docker") {
			continue
		}

		rest := strings.Fields(line[colonIdx+1:])
		if len(rest) < 10 {
			continue
		}

		rxBytes := parseUint64(rest[0])
		txBytes := parseUint64(rest[8])

		current[name] = netCounters{rxBytes: rxBytes, txBytes: txBytes, at: now}

		iface := InterfaceStats{Name: name}
		if p, ok := prev[name]; ok {
			elapsed := now.Sub(p.at).Seconds()
			// Guard against counter wrap-around or reset
			if elapsed > 0 && rxBytes >= p.rxBytes && txBytes >= p.txBytes {
				iface.RxBytesPerSec = uint64(float64(rxBytes-p.rxBytes) / elapsed)
				iface.TxBytesPerSec = uint64(float64(txBytes-p.txBytes) / elapsed)
			}
		}
		stats.Interfaces = append(stats.Interfaces, iface)
	}

	return stats, current
}
