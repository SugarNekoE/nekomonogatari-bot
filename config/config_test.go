package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadConfigFile(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.yaml")
	contents := `
language: zh
telegram:
  token: "123456:test-token"
  username: "@NekoRegisterBot"
  allowed-groups: [-1001, -1002]
database:
  path: test.db
plugins:
  asnk-forge:
    enabled: true
    listen-address: 127.0.0.1:8081
    public-url: http://localhost:8081
    forgejo-url: https://forge.example.test
    forgejo-api-token: secret
    pending-ttl: 20m
    session-ttl: 10m
    telegram-auth-max-age: 3m
    http-timeout: 4s
  mc-whitelist:
    enabled: true
    address: 127.0.0.1:25575
    password: rcon-secret
    timeout: 2s
  system-status:
    enabled: true
    disk-path: /srv
    show-hostname: true
    timeout: 1500ms
`
	if err := os.WriteFile(configFile, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_PATH", configFile)
	// A locale LANGUAGE variable must not override the bot's language.
	t.Setenv("LANGUAGE", "en_US.UTF-8")

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Language != LanguageChinese {
		t.Errorf("Language = %q", got.Language)
	}
	if got.Telegram.Username != "NekoRegisterBot" {
		t.Errorf("Telegram.Username = %q", got.Telegram.Username)
	}
	if got.Plugins.AsnkForge.PendingTTL != 20*time.Minute {
		t.Errorf("PendingTTL = %s", got.Plugins.AsnkForge.PendingTTL)
	}
	if got.Plugins.MCWhitelist.Timeout != 2*time.Second {
		t.Errorf("MC timeout = %s", got.Plugins.MCWhitelist.Timeout)
	}
	if got.Plugins.SystemStatus.DiskPath != "/srv" || !got.Plugins.SystemStatus.ShowHostname || got.Plugins.SystemStatus.Timeout != 1500*time.Millisecond {
		t.Errorf("System status config = %#v", got.Plugins.SystemStatus)
	}
}

func TestLoadEnvironmentOnly(t *testing.T) {
	t.Setenv("CONFIG_PATH", t.TempDir())
	t.Setenv("LANGUAGE", "en_US.UTF-8")
	t.Setenv("NEKOMONOGATARI_LANGUAGE", "zh")
	t.Setenv("TELEGRAM_TOKEN", "123456:env-token")
	t.Setenv("TELEGRAM_ALLOWED_GROUPS", "-1001,-1002")
	t.Setenv("DATABASE_PATH", "env.db")

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Language != LanguageChinese {
		t.Errorf("Language = %q", got.Language)
	}
	if got.Telegram.Token != "123456:env-token" {
		t.Errorf("Telegram.Token = %q", got.Telegram.Token)
	}
	if len(got.Telegram.AllowedGroups) != 2 {
		t.Errorf("AllowedGroups = %v", got.Telegram.AllowedGroups)
	}
}

func TestLoadMissingExplicitConfigFile(t *testing.T) {
	t.Setenv("CONFIG_PATH", filepath.Join(t.TempDir(), "missing.yaml"))
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "read config") {
		t.Fatalf("Load() error = %v, want missing config error", err)
	}
}

func TestValidateRejectsInvalidPluginConfig(t *testing.T) {
	cfg := Config{
		Language: LanguageEnglish,
		Telegram: TelegramConfig{Token: "123456:test-token", Username: "NekoRegisterBot"},
		Database: DatabaseConfig{Path: "test.db"},
		Plugins: PluginsConfig{AsnkForge: AsnkForgeConfig{
			Enabled:            true,
			ListenAddress:      ":8080",
			PublicURL:          "http://public.example.test",
			ForgejoURL:         "https://forge.example.test",
			ForgejoAPIToken:    "secret",
			PendingTTL:         time.Minute,
			SessionTTL:         time.Minute,
			TelegramAuthMaxAge: time.Minute,
			HTTPTimeout:        time.Second,
		}},
	}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("Validate() error = %v, want HTTPS error", err)
	}
}

func TestValidateRequiresForgejoHTTPSOptIn(t *testing.T) {
	cfg := Config{
		Language: LanguageEnglish,
		Telegram: TelegramConfig{Token: "123456:test-token", Username: "NekoRegisterBot"},
		Database: DatabaseConfig{Path: "test.db"},
		Plugins: PluginsConfig{AsnkForge: AsnkForgeConfig{
			Enabled:            true,
			ListenAddress:      ":8080",
			PublicURL:          "https://register.example.test",
			ForgejoURL:         "http://forge.internal:3000",
			ForgejoAPIToken:    "secret",
			PendingTTL:         time.Minute,
			SessionTTL:         time.Minute,
			TelegramAuthMaxAge: time.Minute,
			HTTPTimeout:        time.Second,
		}},
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "allow-insecure-forgejo") {
		t.Fatalf("Validate() error = %v", err)
	}
	cfg.Plugins.AsnkForge.AllowInsecureForgejo = true
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() with opt-in error = %v", err)
	}
}

func TestValidateSystemStatus(t *testing.T) {
	cfg := Config{
		Language: LanguageEnglish,
		Telegram: TelegramConfig{Token: "123456:test-token", Username: "NekoRegisterBot"},
		Database: DatabaseConfig{Path: "test.db"},
		Plugins: PluginsConfig{SystemStatus: SystemStatusConfig{
			Enabled:  true,
			DiskPath: "/",
			Timeout:  time.Second,
		}},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid system status config: %v", err)
	}
	cfg.Plugins.SystemStatus.DiskPath = ""
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "disk-path") {
		t.Fatalf("empty disk path error = %v", err)
	}
	cfg.Plugins.SystemStatus.DiskPath = "/"
	cfg.Plugins.SystemStatus.Timeout = 499 * time.Millisecond
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "500ms") {
		t.Fatalf("short timeout error = %v", err)
	}
}
