package health

import (
	"encoding/json"
	"os/exec"
	"strings"
)

func collectDocker() *DockerStats {
	out, err := exec.Command("docker", "ps", "-a", "--format", "{{json .}}").Output()
	if err != nil {
		return nil
	}

	stats := &DockerStats{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		var container struct {
			Names  string `json:"Names"`
			Image  string `json:"Image"`
			State  string `json:"State"`
			Status string `json:"Status"`
		}
		if err := json.Unmarshal([]byte(line), &container); err != nil {
			continue
		}
		stats.Containers = append(stats.Containers, ContainerInfo{
			Name:   container.Names,
			Image:  container.Image,
			State:  container.State,
			Status: container.Status,
		})
	}
	return stats
}
