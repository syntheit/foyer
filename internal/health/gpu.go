package health

import (
	"os/exec"
	"strings"
)

func collectGPU() *GPUStats {
	out, err := exec.Command(
		"nvidia-smi",
		"--query-gpu=name,utilization.gpu,memory.used,memory.total,temperature.gpu,power.draw",
		"--format=csv,noheader,nounits",
	).Output()
	if err != nil {
		return nil
	}

	line := strings.TrimSpace(string(out))
	if line == "" {
		return nil
	}

	// Handle multi-GPU: take only first line
	if idx := strings.IndexByte(line, '\n'); idx >= 0 {
		line = line[:idx]
	}

	parts := strings.Split(line, ",")
	if len(parts) < 6 {
		return nil
	}

	// Trim each field individually to handle varying separator styles
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	return &GPUStats{
		Name:               parts[0],
		UtilizationPercent: parseFloat64(parts[1]),
		MemoryUsedMB:       parseUint64(parts[2]),
		MemoryTotalMB:      parseUint64(parts[3]),
		Temperature:        int(parseUint64(parts[4])),
		PowerWatts:         parseFloat64(parts[5]),
	}
}
