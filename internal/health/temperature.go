package health

import (
	"os"
	"path/filepath"
	"strings"
)

func collectTemperatures() TempStats {
	stats := TempStats{}

	// Read from thermal zones
	zones, err := filepath.Glob("/sys/class/thermal/thermal_zone*/temp")
	if err != nil {
		return stats
	}

	for _, zone := range zones {
		data, err := os.ReadFile(zone)
		if err != nil {
			continue
		}
		temp := int(parseUint64(strings.TrimSpace(string(data)))) / 1000

		// Check the type to identify CPU
		typeFile := filepath.Join(filepath.Dir(zone), "type")
		typeData, err := os.ReadFile(typeFile)
		if err != nil {
			continue
		}
		typeName := strings.TrimSpace(string(typeData))

		if strings.Contains(typeName, "x86_pkg") || strings.Contains(typeName, "coretemp") || typeName == "cpu" {
			if temp > stats.CPU {
				stats.CPU = temp
			}
		}
	}

	// Also try hwmon for CPU temp
	if stats.CPU == 0 {
		hwmons, _ := filepath.Glob("/sys/class/hwmon/hwmon*/temp1_input")
		for _, path := range hwmons {
			nameFile := filepath.Join(filepath.Dir(path), "name")
			nameData, err := os.ReadFile(nameFile)
			if err != nil {
				continue
			}
			name := strings.TrimSpace(string(nameData))
			if name == "coretemp" || name == "k10temp" || name == "zenpower" {
				data, err := os.ReadFile(path)
				if err != nil {
					continue
				}
				temp := int(parseUint64(strings.TrimSpace(string(data)))) / 1000
				if temp > stats.CPU {
					stats.CPU = temp
				}
			}
		}
	}

	return stats
}
