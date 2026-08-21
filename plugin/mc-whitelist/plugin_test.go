package mcwhitelist

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forge.asnk.io/sugar/nekomonogatari-bot/config"
	"forge.asnk.io/sugar/nekomonogatari-bot/data"
	"github.com/go-telegram/bot"
)

func TestNewReturnsRegistrationCallbacks(t *testing.T) {
	ctx := context.Background()
	disabled, err := New(ctx, config.LanguageEnglish, config.TelegramConfig{Username: "NekoBot"}, config.MCWhitelistConfig{}, nil)
	if err != nil {
		t.Fatalf("disabled New: %v", err)
	}
	if disabled.Register == nil || disabled.Run != nil {
		t.Fatalf("disabled callbacks = %#v", disabled)
	}
	if err := disabled.Register(nil); err != nil {
		t.Fatalf("disabled register: %v", err)
	}

	shared, err := data.Open(ctx, filepath.Join(t.TempDir(), "plugin.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer shared.Close()
	enabled, err := New(ctx, config.LanguageChinese, config.TelegramConfig{Username: "NekoBot"}, config.MCWhitelistConfig{
		Enabled:  true,
		Address:  "127.0.0.1:25575",
		Password: "secret",
		Timeout:  time.Second,
	}, shared)
	if err != nil {
		t.Fatalf("enabled New: %v", err)
	}
	if enabled.Register == nil || enabled.Run == nil {
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
	runCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := enabled.Run(runCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Run error = %v", err)
	}
}

func TestNewValidatesConfiguration(t *testing.T) {
	ctx := context.Background()
	shared, err := data.Open(ctx, filepath.Join(t.TempDir(), "plugin.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer shared.Close()
	valid := config.MCWhitelistConfig{Enabled: true, Address: "127.0.0.1:25575", Password: "secret", Timeout: time.Second}
	tests := []struct {
		name     string
		language config.Language
		cfg      config.MCWhitelistConfig
		shared   *data.Store
	}{
		{name: "language", language: "jp", cfg: valid, shared: shared},
		{name: "store", language: config.LanguageEnglish, cfg: valid},
		{name: "address", language: config.LanguageEnglish, cfg: config.MCWhitelistConfig{Enabled: true, Password: "secret", Timeout: time.Second}, shared: shared},
		{name: "malformed address", language: config.LanguageEnglish, cfg: config.MCWhitelistConfig{Enabled: true, Address: "localhost", Password: "secret", Timeout: time.Second}, shared: shared},
		{name: "password", language: config.LanguageEnglish, cfg: config.MCWhitelistConfig{Enabled: true, Address: "host:1", Timeout: time.Second}, shared: shared},
		{name: "password NUL", language: config.LanguageEnglish, cfg: config.MCWhitelistConfig{Enabled: true, Address: "host:1", Password: "bad\x00secret", Timeout: time.Second}, shared: shared},
		{name: "password too long", language: config.LanguageEnglish, cfg: config.MCWhitelistConfig{Enabled: true, Address: "host:1", Password: strings.Repeat("x", rconMaxPacketSize), Timeout: time.Second}, shared: shared},
		{name: "timeout", language: config.LanguageEnglish, cfg: config.MCWhitelistConfig{Enabled: true, Address: "host:1", Password: "secret"}, shared: shared},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(ctx, test.language, config.TelegramConfig{Username: "NekoBot"}, test.cfg, test.shared); err == nil {
				t.Fatal("New unexpectedly succeeded")
			}
		})
	}
}
