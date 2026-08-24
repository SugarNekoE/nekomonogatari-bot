package systemstatus

import (
	"context"
	"strings"
	"testing"
	"time"

	"forge.asnk.io/sugar/nekomonogatari-bot/config"
)

func TestSystemCollectorProducesTelegramSafeSnapshot(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	collector := &systemCollector{diskPath: "/", startedAt: time.Now().Add(-time.Minute)}
	snapshot, err := collector.Collect(ctx)
	if err != nil {
		t.Logf("partial host snapshot: %v", err)
	}
	if !snapshot.usable() || snapshot.GoVersion == "" || snapshot.OS == "" || snapshot.KernelArch == "" {
		t.Fatalf("unusable snapshot: %#v", snapshot)
	}
	text := formatStatus(config.LanguageEnglish, snapshot)
	if len([]byte(text)) > 4096 {
		t.Fatalf("status is %d bytes, exceeding Telegram's limit", len([]byte(text)))
	}
}

func TestFormatStatusLanguages(t *testing.T) {
	snapshot := statusSnapshot{
		Hostname:             "status-host",
		OS:                   "linux",
		Platform:             "debian",
		PlatformVersion:      "13",
		KernelVersion:        "6.12.0",
		KernelArch:           "x86_64",
		VirtualizationSystem: "kvm",
		VirtualizationRole:   "guest",
		HostUptime:           49*time.Hour + 2*time.Minute,
		CPUModel:             "Example CPU",
		PhysicalCores:        4,
		LogicalCores:         8,
		CPUInfoAvailable:     true,
		CPUUsage:             12.5,
		CPUUsageAvailable:    true,
		MemoryUsed:           4 * 1024 * 1024 * 1024,
		MemoryTotal:          16 * 1024 * 1024 * 1024,
		MemoryUsage:          25,
		MemoryAvailable:      true,
		DiskPath:             "/",
		DiskUsed:             40 * 1024 * 1024 * 1024,
		DiskTotal:            100 * 1024 * 1024 * 1024,
		DiskUsage:            40,
		DiskAvailable:        true,
		Load1:                0.1,
		Load5:                0.2,
		Load15:               0.3,
		LoadAvailable:        true,
		GoVersion:            "go1.26.0",
		Goroutines:           12,
		BotUptime:            90 * time.Minute,
		Partial:              true,
	}
	tests := []struct {
		language config.Language
		wanted   []string
	}{
		{language: config.LanguageEnglish, wanted: []string{"Server status", "Host: status-host", "System: debian 13 (linux)", "4 cores / 8 threads", "CPU usage: 12.5%", "Memory: 4.0 GiB / 16.0 GiB", "Disk (/): 40.0 GiB / 100.0 GiB", "Some metrics are unavailable"}},
		{language: config.LanguageChinese, wanted: []string{"服务器状态", "主机: status-host", "系统: debian 13 (linux)", "4 核 / 8 线程", "CPU 使用率: 12.5%", "内存: 4.0 GiB / 16.0 GiB", "磁盘 (/): 40.0 GiB / 100.0 GiB", "部分系统指标暂时不可用"}},
	}
	for _, test := range tests {
		t.Run(string(test.language), func(t *testing.T) {
			text := formatStatus(test.language, snapshot)
			for _, wanted := range test.wanted {
				if !strings.Contains(text, wanted) {
					t.Errorf("status does not contain %q:\n%s", wanted, text)
				}
			}
			if strings.Contains(text, "%!") {
				t.Fatalf("status contains formatting error: %s", text)
			}
		})
	}
}

func TestFormattingHelpers(t *testing.T) {
	bytesTests := map[uint64]string{
		0:               "0 B",
		1023:            "1023 B",
		1024:            "1.0 KiB",
		5 * 1024 * 1024: "5.0 MiB",
	}
	for input, wanted := range bytesTests {
		if got := formatBytes(input); got != wanted {
			t.Errorf("formatBytes(%d) = %q; want %q", input, got, wanted)
		}
	}
	if got := formatDuration(config.LanguageEnglish, 25*time.Hour+2*time.Minute); got != "1d 1h 2m" {
		t.Errorf("English duration = %q", got)
	}
	if got := formatDuration(config.LanguageChinese, 25*time.Hour+2*time.Minute); got != "1 天 1 小时 2 分钟" {
		t.Errorf("Chinese duration = %q", got)
	}
	if got := cleanMetric(" CPU\nmodel\tname "); got != "CPU model name" {
		t.Errorf("cleanMetric = %q", got)
	}
	if got := cleanMetric(strings.Repeat("猫", 121)); len([]rune(got)) != 120 || !strings.HasSuffix(got, "…") {
		t.Errorf("truncated cleanMetric has %d runes and suffix %q", len([]rune(got)), got[len(got)-3:])
	}
}
