package systemstatus

import (
	"fmt"
	"strings"
	"time"

	"forge.asnk.io/sugar/nekomonogatari-bot/config"
)

type statusMessages struct {
	title       string
	host        string
	system      string
	kernel      string
	uptime      string
	environment string
	cpu         string
	cpuUsage    string
	load        string
	memory      string
	disk        string
	bot         string
	partial     string
	unavailable string
}

var statusCatalog = map[config.Language]statusMessages{
	config.LanguageEnglish: {
		title:       "🖥 Server status",
		host:        "Host",
		system:      "System",
		kernel:      "Kernel",
		uptime:      "Host uptime",
		environment: "Environment",
		cpu:         "CPU",
		cpuUsage:    "CPU usage",
		load:        "Load (1/5/15m)",
		memory:      "Memory",
		disk:        "Disk",
		bot:         "Bot runtime",
		partial:     "⚠ Some metrics are unavailable.",
		unavailable: "System status is temporarily unavailable. Please try again later.",
	},
	config.LanguageChinese: {
		title:       "🖥 服务器状态",
		host:        "主机",
		system:      "系统",
		kernel:      "内核",
		uptime:      "主机运行时间",
		environment: "运行环境",
		cpu:         "CPU",
		cpuUsage:    "CPU 使用率",
		load:        "负载（1/5/15 分钟）",
		memory:      "内存",
		disk:        "磁盘",
		bot:         "程序状态",
		partial:     "⚠ 部分系统指标暂时不可用。",
		unavailable: "系统状态暂时不可用，请稍后重试。",
	},
}

func formatStatus(language config.Language, snapshot statusSnapshot) string {
	messages, ok := statusCatalog[language]
	if !ok {
		messages = statusCatalog[config.LanguageEnglish]
	}
	lines := []string{messages.title}
	if snapshot.Hostname != "" {
		lines = append(lines, metricLine(messages.host, snapshot.Hostname))
	}
	lines = append(lines, metricLine(messages.system, formatSystem(snapshot)))
	if kernel := joinMetrics(snapshot.KernelVersion, snapshot.KernelArch); kernel != "" {
		lines = append(lines, metricLine(messages.kernel, kernel))
	}
	if snapshot.HostUptime > 0 {
		lines = append(lines, metricLine(messages.uptime, formatDuration(language, snapshot.HostUptime)))
	}
	if environment := joinMetrics(snapshot.VirtualizationSystem, snapshot.VirtualizationRole); environment != "" {
		lines = append(lines, metricLine(messages.environment, environment))
	}
	if snapshot.CPUInfoAvailable {
		lines = append(lines, metricLine(messages.cpu, formatCPU(language, snapshot)))
	}
	if snapshot.CPUUsageAvailable {
		lines = append(lines, metricLine(messages.cpuUsage, formatPercent(snapshot.CPUUsage)))
	}
	if snapshot.LoadAvailable {
		lines = append(lines, metricLine(messages.load, fmt.Sprintf("%.2f / %.2f / %.2f", snapshot.Load1, snapshot.Load5, snapshot.Load15)))
	}
	if snapshot.MemoryAvailable {
		lines = append(lines, metricLine(messages.memory, formatUsage(snapshot.MemoryUsed, snapshot.MemoryTotal, snapshot.MemoryUsage)))
	}
	if snapshot.DiskAvailable {
		label := messages.disk
		if snapshot.DiskPath != "" {
			label += " (" + snapshot.DiskPath + ")"
		}
		lines = append(lines, metricLine(label, formatUsage(snapshot.DiskUsed, snapshot.DiskTotal, snapshot.DiskUsage)))
	}
	if snapshot.GoVersion != "" {
		lines = append(lines, metricLine(messages.bot, formatRuntime(language, snapshot)))
	}
	if snapshot.Partial {
		lines = append(lines, "", messages.partial)
	}
	return strings.Join(lines, "\n")
}

func formatSystem(snapshot statusSnapshot) string {
	platform := joinWords(snapshot.Platform, snapshot.PlatformVersion)
	if platform == "" {
		return snapshot.OS
	}
	if snapshot.OS != "" && !strings.EqualFold(snapshot.OS, snapshot.Platform) {
		return platform + " (" + snapshot.OS + ")"
	}
	return platform
}

func formatCPU(language config.Language, snapshot statusSnapshot) string {
	topology := ""
	if snapshot.PhysicalCores > 0 && snapshot.LogicalCores > 0 {
		if language == config.LanguageChinese {
			topology = fmt.Sprintf("%d 核 / %d 线程", snapshot.PhysicalCores, snapshot.LogicalCores)
		} else {
			topology = fmt.Sprintf("%d cores / %d threads", snapshot.PhysicalCores, snapshot.LogicalCores)
		}
	} else if snapshot.LogicalCores > 0 {
		if language == config.LanguageChinese {
			topology = fmt.Sprintf("%d 线程", snapshot.LogicalCores)
		} else {
			topology = fmt.Sprintf("%d threads", snapshot.LogicalCores)
		}
	} else if snapshot.PhysicalCores > 0 {
		if language == config.LanguageChinese {
			topology = fmt.Sprintf("%d 核", snapshot.PhysicalCores)
		} else {
			topology = fmt.Sprintf("%d cores", snapshot.PhysicalCores)
		}
	}
	return joinMetrics(snapshot.CPUModel, topology)
}

func formatRuntime(language config.Language, snapshot statusSnapshot) string {
	if language == config.LanguageChinese {
		return fmt.Sprintf("%s · %d 个协程 · 已运行 %s", snapshot.GoVersion, snapshot.Goroutines, formatDuration(language, snapshot.BotUptime))
	}
	return fmt.Sprintf("%s · %d goroutines · uptime %s", snapshot.GoVersion, snapshot.Goroutines, formatDuration(language, snapshot.BotUptime))
}

func metricLine(label, value string) string {
	return label + ": " + value
}

func formatUsage(used, total uint64, percentage float64) string {
	return fmt.Sprintf("%s / %s (%s)", formatBytes(used), formatBytes(total), formatPercent(percentage))
}

func formatPercent(value float64) string {
	return fmt.Sprintf("%.1f%%", value)
}

func formatBytes(value uint64) string {
	const unit = uint64(1024)
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	divisor := unit
	exponent := 0
	for amount := value / unit; amount >= unit && exponent < 5; amount /= unit {
		divisor *= unit
		exponent++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(divisor), "KMGTPE"[exponent])
}

func formatDuration(language config.Language, duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}
	days := int(duration / (24 * time.Hour))
	duration %= 24 * time.Hour
	hours := int(duration / time.Hour)
	duration %= time.Hour
	minutes := int(duration / time.Minute)
	seconds := int((duration % time.Minute) / time.Second)
	var parts []string
	if language == config.LanguageChinese {
		if days > 0 {
			parts = append(parts, fmt.Sprintf("%d 天", days))
		}
		if hours > 0 {
			parts = append(parts, fmt.Sprintf("%d 小时", hours))
		}
		if minutes > 0 {
			parts = append(parts, fmt.Sprintf("%d 分钟", minutes))
		}
		if len(parts) == 0 {
			parts = append(parts, fmt.Sprintf("%d 秒", seconds))
		}
	} else {
		if days > 0 {
			parts = append(parts, fmt.Sprintf("%dd", days))
		}
		if hours > 0 {
			parts = append(parts, fmt.Sprintf("%dh", hours))
		}
		if minutes > 0 {
			parts = append(parts, fmt.Sprintf("%dm", minutes))
		}
		if len(parts) == 0 {
			parts = append(parts, fmt.Sprintf("%ds", seconds))
		}
	}
	return strings.Join(parts, " ")
}

func joinWords(values ...string) string {
	var present []string
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			present = append(present, value)
		}
	}
	return strings.Join(present, " ")
}

func joinMetrics(values ...string) string {
	var present []string
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			present = append(present, value)
		}
	}
	return strings.Join(present, " · ")
}
