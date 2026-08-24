package systemstatus

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"forge.asnk.io/sugar/nekomonogatari-bot/config"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type fakeCollector struct {
	snapshot    statusSnapshot
	err         error
	calls       int
	hadDeadline bool
}

func (f *fakeCollector) Collect(ctx context.Context) (statusSnapshot, error) {
	f.calls++
	_, f.hadDeadline = ctx.Deadline()
	return f.snapshot, f.err
}

type telegramCall struct {
	method string
	values map[string][]string
}

type fakeTelegramClient struct {
	mu    sync.Mutex
	calls []telegramCall
}

func (f *fakeTelegramClient) Do(request *http.Request) (*http.Response, error) {
	if err := request.ParseMultipartForm(1 << 20); err != nil {
		return nil, err
	}
	values := make(map[string][]string, len(request.MultipartForm.Value))
	for key, value := range request.MultipartForm.Value {
		values[key] = append([]string(nil), value...)
	}
	method := path.Base(request.URL.Path)
	f.mu.Lock()
	f.calls = append(f.calls, telegramCall{method: method, values: values})
	f.mu.Unlock()
	result := "true"
	if method == "sendMessage" {
		result = `{"message_id":77,"date":1,"chat":{"id":-1001,"type":"supergroup"},"text":"ok"}`
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"ok":true,"result":` + result + `}`)),
	}, nil
}

func (f *fakeTelegramClient) takeCalls() []telegramCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	calls := append([]telegramCall(nil), f.calls...)
	f.calls = nil
	return calls
}

func TestNewAndRegister(t *testing.T) {
	disabled, err := New(config.LanguageEnglish, config.TelegramConfig{}, config.SystemStatusConfig{})
	if err != nil {
		t.Fatalf("disabled New: %v", err)
	}
	if disabled.Register == nil || disabled.Run != nil {
		t.Fatalf("disabled callbacks = %#v", disabled)
	}
	if err := disabled.Register(nil); err != nil {
		t.Fatalf("disabled register: %v", err)
	}

	valid := config.SystemStatusConfig{Enabled: true, DiskPath: "/", Timeout: time.Second}
	enabled, err := New(config.LanguageChinese, config.TelegramConfig{Username: "NekoBot"}, valid)
	if err != nil {
		t.Fatalf("enabled New: %v", err)
	}
	if enabled.Register == nil || enabled.Run != nil {
		t.Fatalf("enabled callbacks = %#v", enabled)
	}
	if err := enabled.Register(nil); err == nil {
		t.Fatal("enabled Register(nil) unexpectedly succeeded")
	}
	b, err := bot.New("test-token", bot.WithSkipGetMe())
	if err != nil {
		t.Fatal(err)
	}
	if err := enabled.Register(b); err != nil {
		t.Fatalf("register handlers: %v", err)
	}
}

func TestNewValidatesConfiguration(t *testing.T) {
	valid := config.SystemStatusConfig{Enabled: true, DiskPath: "/", Timeout: time.Second}
	tests := []struct {
		name     string
		language config.Language
		username string
		cfg      config.SystemStatusConfig
	}{
		{name: "language", language: "jp", username: "NekoBot", cfg: valid},
		{name: "username", language: config.LanguageEnglish, cfg: valid},
		{name: "disk path", language: config.LanguageEnglish, username: "NekoBot", cfg: config.SystemStatusConfig{Enabled: true, Timeout: time.Second}},
		{name: "timeout", language: config.LanguageEnglish, username: "NekoBot", cfg: config.SystemStatusConfig{Enabled: true, DiskPath: "/", Timeout: 499 * time.Millisecond}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.language, config.TelegramConfig{Username: test.username}, test.cfg); err == nil {
				t.Fatal("New unexpectedly succeeded")
			}
		})
	}
}

func TestStatusCommandSendsLocalizedPartialSnapshot(t *testing.T) {
	collector := &fakeCollector{
		snapshot: statusSnapshot{
			OS:                "linux",
			KernelArch:        "x86_64",
			CPUModel:          "Example CPU",
			LogicalCores:      4,
			CPUInfoAvailable:  true,
			CPUUsage:          7.5,
			CPUUsageAvailable: true,
			MemoryUsed:        1024,
			MemoryTotal:       4096,
			MemoryUsage:       25,
			MemoryAvailable:   true,
			GoVersion:         "go1.26.0",
			Goroutines:        8,
			BotUptime:         time.Minute,
			Partial:           true,
		},
		err: errors.New("disk unavailable"),
	}
	p := &plugin{
		enabled:     true,
		language:    config.LanguageChinese,
		botUsername: "NekoBot",
		timeout:     time.Second,
		collector:   collector,
	}
	client := &fakeTelegramClient{}
	b, err := bot.New(
		"test-token",
		bot.WithSkipGetMe(),
		bot.WithNotAsyncHandlers(),
		bot.WithHTTPClient(time.Second, client),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.register(b); err != nil {
		t.Fatal(err)
	}
	b.ProcessUpdate(context.Background(), &models.Update{Message: &models.Message{
		ID:              10,
		MessageThreadID: 3,
		Text:            "/status@NekoBot",
		From:            &models.User{ID: 101, FirstName: "User"},
		Chat:            models.Chat{ID: -1001, Type: models.ChatTypeSupergroup},
	}})
	calls := client.takeCalls()
	if len(calls) != 1 || calls[0].method != "sendMessage" {
		t.Fatalf("status calls = %#v", calls)
	}
	text := calls[0].values["text"][0]
	for _, wanted := range []string{"服务器状态", "Example CPU", "CPU 使用率: 7.5%", "部分系统指标暂时不可用"} {
		if !strings.Contains(text, wanted) {
			t.Errorf("status text does not contain %q: %s", wanted, text)
		}
	}
	if calls[0].values["message_thread_id"][0] != "3" || !strings.Contains(calls[0].values["reply_parameters"][0], `"message_id":10`) {
		t.Errorf("reply metadata = %#v", calls[0].values)
	}
	if collector.calls != 1 || !collector.hadDeadline {
		t.Errorf("collector calls=%d deadline=%v", collector.calls, collector.hadDeadline)
	}
}

func TestStatusCommandUnavailableAndIgnoresBots(t *testing.T) {
	collector := &fakeCollector{err: errors.New("unavailable")}
	p := &plugin{enabled: true, language: config.LanguageEnglish, botUsername: "NekoBot", timeout: time.Second, collector: collector}
	client := &fakeTelegramClient{}
	b, err := bot.New("test-token", bot.WithSkipGetMe(), bot.WithNotAsyncHandlers(), bot.WithHTTPClient(time.Second, client))
	if err != nil {
		t.Fatal(err)
	}
	if err := p.register(b); err != nil {
		t.Fatal(err)
	}
	b.ProcessUpdate(context.Background(), &models.Update{Message: &models.Message{
		ID: 11, Text: "/status", From: &models.User{ID: 101}, Chat: models.Chat{ID: -1001, Type: models.ChatTypeSupergroup},
	}})
	calls := client.takeCalls()
	if len(calls) != 1 || !strings.Contains(calls[0].values["text"][0], "temporarily unavailable") {
		t.Fatalf("unavailable calls = %#v", calls)
	}
	b.ProcessUpdate(context.Background(), &models.Update{Message: &models.Message{
		ID: 12, Text: "/status", From: &models.User{ID: 102, IsBot: true}, Chat: models.Chat{ID: -1001, Type: models.ChatTypeSupergroup},
	}})
	if calls := client.takeCalls(); len(calls) != 0 {
		t.Fatalf("bot command calls = %#v", calls)
	}
}
