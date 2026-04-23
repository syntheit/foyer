package health

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const (
	collectInterval    = 5 * time.Second
	dockerInterval     = 30 * time.Second
	networkHistorySize = 360 // 30 minutes at 5-second intervals
)

// Stats is the full system health snapshot.
type Stats struct {
	Timestamp    time.Time    `json:"timestamp"`
	CPU          CPUStats     `json:"cpu"`
	Memory       MemoryStats  `json:"memory"`
	Disk         DiskStats    `json:"disk"`
	Network      NetworkStats `json:"network"`
	Temperatures TempStats    `json:"temperatures"`
	GPU          *GPUStats    `json:"gpu"`
	Docker       *DockerStats `json:"docker"`
	System       SystemStats  `json:"system"`
}

type CPUStats struct {
	UsagePercent float64   `json:"usage_percent"`
	Cores        int       `json:"cores"`
	Load         []float64 `json:"load"`
}

type MemoryStats struct {
	TotalBytes        uint64  `json:"total_bytes"`
	UsedBytes         uint64  `json:"used_bytes"`
	AvailableBytes    uint64  `json:"available_bytes"`
	UsagePercent      float64 `json:"usage_percent"`
	CompressedPercent float64 `json:"compressed_percent"`
}

type DiskStats struct {
	Pools  []PoolStats  `json:"pools"`
	Mounts []MountStats `json:"mounts"`
}

type PoolStats struct {
	Name         string  `json:"name"`
	TotalBytes   uint64  `json:"total_bytes"`
	UsedBytes    uint64  `json:"used_bytes"`
	FreeBytes    uint64  `json:"free_bytes"`
	UsagePercent float64 `json:"usage_percent"`
	Health       string  `json:"health"`
}

type MountStats struct {
	Mountpoint   string  `json:"mountpoint"`
	Filesystem   string  `json:"filesystem"`
	TotalBytes   uint64  `json:"total_bytes"`
	UsedBytes    uint64  `json:"used_bytes"`
	UsagePercent float64 `json:"usage_percent"`
}

type NetworkStats struct {
	Interfaces []InterfaceStats `json:"interfaces"`
}

type InterfaceStats struct {
	Name          string `json:"name"`
	RxBytesPerSec uint64 `json:"rx_bytes_per_sec"`
	TxBytesPerSec uint64 `json:"tx_bytes_per_sec"`
}

type NetworkHistoryEntry struct {
	Timestamp  time.Time        `json:"timestamp"`
	Interfaces []InterfaceStats `json:"interfaces"`
}

type TempStats struct {
	CPU int `json:"cpu"`
	GPU int `json:"gpu"`
}

type GPUStats struct {
	Name               string  `json:"name"`
	UtilizationPercent float64 `json:"utilization_percent"`
	MemoryUsedMB       uint64  `json:"memory_used_mb"`
	MemoryTotalMB      uint64  `json:"memory_total_mb"`
	Temperature        int     `json:"temperature"`
	PowerWatts         float64 `json:"power_watts"`
}

type DockerStats struct {
	Containers []ContainerInfo `json:"containers"`
}

type ContainerInfo struct {
	Name   string `json:"name"`
	Image  string `json:"image"`
	State  string `json:"state"`
	Status string `json:"status"`
}

type SystemStats struct {
	Hostname      string    `json:"hostname"`
	Kernel        string    `json:"kernel"`
	UptimeSeconds uint64    `json:"uptime_seconds"`
	LoadAvg       []float64 `json:"load_avg"`
}

// Collector gathers system metrics on a regular interval.
// The collect method is only called from the Run goroutine, so prevCPU/prevNet
// do not need synchronization. Only `current` and `networkHistory` are read
// externally and are protected by mu.
type Collector struct {
	mu             sync.RWMutex
	current        Stats
	networkHistory []NetworkHistoryEntry

	// Internal state — only accessed from the collect goroutine
	prevCPU            *cpuTicks
	prevNet            map[string]netCounters
	lastDocker         time.Time
	cachedDocker       *DockerStats
	temperatureCommand string
	lastTempCmd        time.Time
	cachedTempCmd      TempStats
}

func NewCollector(temperatureCommand string) *Collector {
	return &Collector{
		networkHistory:     make([]NetworkHistoryEntry, 0, networkHistorySize),
		prevNet:            make(map[string]netCounters),
		temperatureCommand: temperatureCommand,
	}
}

// Current returns a copy of the latest stats snapshot.
func (c *Collector) Current() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.current
}

// NetworkHistory returns the network history ring buffer.
func (c *Collector) NetworkHistory() []NetworkHistoryEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make([]NetworkHistoryEntry, len(c.networkHistory))
	copy(result, c.networkHistory)
	return result
}

func (c *Collector) Run(ctx context.Context) {
	c.collect()
	ticker := time.NewTicker(collectInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.collect()
		}
	}
}

func (c *Collector) collect() {
	now := time.Now()
	stats := Stats{Timestamp: now}

	stats.CPU, c.prevCPU = collectCPU(c.prevCPU)
	stats.Memory = collectMemory()
	stats.Disk = collectDisk()
	stats.Network, c.prevNet = collectNetwork(c.prevNet)
	if c.temperatureCommand != "" {
		if time.Since(c.lastTempCmd) >= dockerInterval {
			c.cachedTempCmd = collectTemperatureCommand(c.temperatureCommand)
			c.lastTempCmd = now
		}
		stats.Temperatures = c.cachedTempCmd
	} else {
		stats.Temperatures = collectTemperatures()
	}
	stats.GPU = collectGPU()
	if stats.GPU != nil {
		stats.Temperatures.GPU = stats.GPU.Temperature
	}
	stats.System = collectSystem()

	// Collect load once, populate both CPU and System
	load := readLoadAvg()
	stats.CPU.Load = load
	stats.System.LoadAvg = load

	// Docker: collect less frequently
	if time.Since(c.lastDocker) >= dockerInterval {
		c.cachedDocker = collectDocker()
		c.lastDocker = now
	}
	stats.Docker = c.cachedDocker

	c.mu.Lock()
	c.current = stats
	entry := NetworkHistoryEntry{Timestamp: now, Interfaces: stats.Network.Interfaces}
	if len(c.networkHistory) >= networkHistorySize {
		c.networkHistory = c.networkHistory[1:]
	}
	c.networkHistory = append(c.networkHistory, entry)
	c.mu.Unlock()

	slog.Debug("collected stats", "cpu", stats.CPU.UsagePercent, "mem", stats.Memory.UsagePercent)
}
