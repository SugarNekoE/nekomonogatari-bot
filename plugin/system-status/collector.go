package systemstatus

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"
	"unicode"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
)

const cpuSampleDuration = 200 * time.Millisecond

type statusSnapshot struct {
	Hostname             string
	OS                   string
	Platform             string
	PlatformVersion      string
	KernelVersion        string
	KernelArch           string
	VirtualizationSystem string
	VirtualizationRole   string
	HostUptime           time.Duration

	CPUModel          string
	PhysicalCores     int
	LogicalCores      int
	CPUInfoAvailable  bool
	CPUUsage          float64
	CPUUsageAvailable bool

	MemoryTotal     uint64
	MemoryUsed      uint64
	MemoryUsage     float64
	MemoryAvailable bool

	DiskPath      string
	DiskTotal     uint64
	DiskUsed      uint64
	DiskUsage     float64
	DiskAvailable bool

	Load1         float64
	Load5         float64
	Load15        float64
	LoadAvailable bool

	GoVersion  string
	Goroutines int
	BotUptime  time.Duration
	Partial    bool
}

func (s statusSnapshot) usable() bool {
	return s.GoVersion != "" || s.CPUInfoAvailable || s.CPUUsageAvailable || s.MemoryAvailable || s.DiskAvailable
}

type collector interface {
	Collect(context.Context) (statusSnapshot, error)
}

type systemCollector struct {
	diskPath     string
	showHostname bool
	startedAt    time.Time
	now          func() time.Time
}

func (c *systemCollector) Collect(ctx context.Context) (statusSnapshot, error) {
	now := time.Now
	if c.now != nil {
		now = c.now
	}
	snapshot := statusSnapshot{
		OS:         runtime.GOOS,
		KernelArch: runtime.GOARCH,
		DiskPath:   c.diskPath,
		GoVersion:  runtime.Version(),
		Goroutines: runtime.NumGoroutine(),
		BotUptime:  max(0, now().Sub(c.startedAt)),
	}
	var collectionErrors []error

	if info, err := host.InfoWithContext(ctx); err != nil {
		collectionErrors = append(collectionErrors, fmt.Errorf("host: %w", err))
	} else {
		if c.showHostname {
			snapshot.Hostname = cleanMetric(info.Hostname)
		}
		snapshot.OS = firstMetric(info.OS, snapshot.OS)
		snapshot.Platform = cleanMetric(info.Platform)
		snapshot.PlatformVersion = cleanMetric(info.PlatformVersion)
		snapshot.KernelVersion = cleanMetric(info.KernelVersion)
		snapshot.KernelArch = firstMetric(info.KernelArch, snapshot.KernelArch)
		snapshot.VirtualizationSystem = cleanMetric(info.VirtualizationSystem)
		snapshot.VirtualizationRole = cleanMetric(info.VirtualizationRole)
		snapshot.HostUptime = time.Duration(info.Uptime) * time.Second
	}

	if info, err := cpu.InfoWithContext(ctx); err != nil {
		collectionErrors = append(collectionErrors, fmt.Errorf("cpu info: %w", err))
	} else {
		for _, item := range info {
			if model := cleanMetric(item.ModelName); model != "" {
				snapshot.CPUModel = model
				break
			}
		}
		snapshot.CPUInfoAvailable = len(info) > 0
	}
	if count, err := cpu.CountsWithContext(ctx, false); err != nil {
		collectionErrors = append(collectionErrors, fmt.Errorf("physical CPU count: %w", err))
	} else {
		snapshot.PhysicalCores = count
		snapshot.CPUInfoAvailable = snapshot.CPUInfoAvailable || count > 0
	}
	if count, err := cpu.CountsWithContext(ctx, true); err != nil {
		collectionErrors = append(collectionErrors, fmt.Errorf("logical CPU count: %w", err))
	} else {
		snapshot.LogicalCores = count
		snapshot.CPUInfoAvailable = snapshot.CPUInfoAvailable || count > 0
	}
	if percentages, err := cpu.PercentWithContext(ctx, cpuSampleDuration, false); err != nil {
		collectionErrors = append(collectionErrors, fmt.Errorf("CPU usage: %w", err))
	} else if len(percentages) > 0 {
		snapshot.CPUUsage = percentages[0]
		snapshot.CPUUsageAvailable = true
	}

	if memory, err := mem.VirtualMemoryWithContext(ctx); err != nil {
		collectionErrors = append(collectionErrors, fmt.Errorf("memory: %w", err))
	} else {
		snapshot.MemoryTotal = memory.Total
		snapshot.MemoryUsed = memory.Used
		snapshot.MemoryUsage = memory.UsedPercent
		snapshot.MemoryAvailable = true
	}

	if usage, err := disk.UsageWithContext(ctx, c.diskPath); err != nil {
		collectionErrors = append(collectionErrors, fmt.Errorf("disk %q: %w", c.diskPath, err))
	} else {
		snapshot.DiskTotal = usage.Total
		snapshot.DiskUsed = usage.Used
		snapshot.DiskUsage = usage.UsedPercent
		snapshot.DiskAvailable = true
	}

	if average, err := load.AvgWithContext(ctx); err != nil {
		collectionErrors = append(collectionErrors, fmt.Errorf("load average: %w", err))
	} else {
		snapshot.Load1 = average.Load1
		snapshot.Load5 = average.Load5
		snapshot.Load15 = average.Load15
		snapshot.LoadAvailable = true
	}

	snapshot.OS = cleanMetric(snapshot.OS)
	snapshot.KernelArch = cleanMetric(snapshot.KernelArch)
	snapshot.DiskPath = cleanMetric(snapshot.DiskPath)
	snapshot.Partial = len(collectionErrors) > 0
	return snapshot, errors.Join(collectionErrors...)
}

func firstMetric(value, fallback string) string {
	if cleaned := cleanMetric(value); cleaned != "" {
		return cleaned
	}
	return cleanMetric(fallback)
}

func cleanMetric(value string) string {
	value = strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return ' '
		}
		return character
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > 120 {
		value = string(runes[:119]) + "…"
	}
	return value
}
