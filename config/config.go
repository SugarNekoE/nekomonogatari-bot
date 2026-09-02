package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const placeholderToken = "BOT_TOKEN_FROM_BOTFATHER"

var telegramUsernamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{4,31}$`)

func Load() (*Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	if configPath := strings.TrimSpace(os.Getenv("CONFIG_PATH")); configPath != "" {
		info, err := os.Stat(configPath)
		switch {
		case err == nil && info.IsDir():
			v.SetConfigName("config")
			v.AddConfigPath(configPath)
		case err == nil:
			v.SetConfigFile(configPath)
		case errors.Is(err, os.ErrNotExist):
			v.SetConfigFile(configPath)
		default:
			return nil, fmt.Errorf("inspect config path: %w", err)
		}
	} else {
		v.SetConfigName("config")
		v.AddConfigPath(".")
	}
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	setDefaults(v)

	if err := bindEnvironment(v); err != nil {
		return nil, fmt.Errorf("bind environment: %w", err)
	}
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return nil, fmt.Errorf("read config: %w", err)
		}
	}

	conf := &Config{}
	if err := v.Unmarshal(conf); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	conf.Telegram.Username = strings.TrimPrefix(strings.TrimSpace(conf.Telegram.Username), "@")
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	return conf, nil
}

func bindEnvironment(v *viper.Viper) error {
	keys := []string{
		"language",
		"telegram.token",
		"telegram.username",
		"telegram.allowed-groups",
		"database.path",
		"plugins.asnk-forge.enabled",
		"plugins.asnk-forge.listen-address",
		"plugins.asnk-forge.public-url",
		"plugins.asnk-forge.forgejo-url",
		"plugins.asnk-forge.forgejo-api-token",
		"plugins.asnk-forge.allow-insecure-forgejo",
		"plugins.asnk-forge.pending-ttl",
		"plugins.asnk-forge.session-ttl",
		"plugins.asnk-forge.telegram-auth-max-age",
		"plugins.asnk-forge.http-timeout",
		"plugins.mc-whitelist.enabled",
		"plugins.mc-whitelist.address",
		"plugins.mc-whitelist.password",
		"plugins.mc-whitelist.timeout",
		"plugins.system-status.enabled",
		"plugins.system-status.disk-path",
		"plugins.system-status.show-hostname",
		"plugins.system-status.timeout",
	}
	for _, key := range keys {
		var err error
		if key == "language" {
			// LANGUAGE is commonly set by the operating system for locale
			// selection, so do not accidentally interpret it as bot config.
			err = v.BindEnv(key, "NEKOMONOGATARI_LANGUAGE")
		} else {
			err = v.BindEnv(key)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("language", string(LanguageEnglish))
	v.SetDefault("telegram.token", placeholderToken)
	v.SetDefault("telegram.username", "")
	v.SetDefault("telegram.allowed-groups", []int64{})
	v.SetDefault("database.path", "data/nekomonogatari.db")

	v.SetDefault("plugins.asnk-forge.enabled", false)
	v.SetDefault("plugins.asnk-forge.listen-address", "127.0.0.1:8080")
	v.SetDefault("plugins.asnk-forge.public-url", "http://localhost:8080")
	v.SetDefault("plugins.asnk-forge.forgejo-url", "")
	v.SetDefault("plugins.asnk-forge.forgejo-api-token", "")
	v.SetDefault("plugins.asnk-forge.allow-insecure-forgejo", false)
	v.SetDefault("plugins.asnk-forge.pending-ttl", 15*time.Minute)
	v.SetDefault("plugins.asnk-forge.session-ttl", 15*time.Minute)
	v.SetDefault("plugins.asnk-forge.telegram-auth-max-age", 5*time.Minute)
	v.SetDefault("plugins.asnk-forge.http-timeout", 10*time.Second)

	v.SetDefault("plugins.mc-whitelist.enabled", false)
	v.SetDefault("plugins.mc-whitelist.address", "127.0.0.1:25575")
	v.SetDefault("plugins.mc-whitelist.password", "")
	v.SetDefault("plugins.mc-whitelist.timeout", 5*time.Second)

	v.SetDefault("plugins.system-status.enabled", false)
	v.SetDefault("plugins.system-status.disk-path", "/")
	v.SetDefault("plugins.system-status.show-hostname", false)
	v.SetDefault("plugins.system-status.timeout", 3*time.Second)
}

func (c *Config) Validate() error {
	if c.Language != LanguageEnglish && c.Language != LanguageChinese {
		return fmt.Errorf("language must be %q or %q", LanguageEnglish, LanguageChinese)
	}
	if token := strings.TrimSpace(c.Telegram.Token); token == "" || token == placeholderToken {
		return errors.New("telegram.token must be configured")
	}
	if strings.TrimSpace(c.Database.Path) == "" {
		return errors.New("database.path must not be empty")
	}
	if hasDuplicates(c.Telegram.AllowedGroups) {
		return errors.New("telegram.allowed-groups must not contain duplicate chat IDs")
	}
	if c.Plugins.AsnkForge.Enabled || c.Plugins.MCWhitelist.Enabled || c.Plugins.SystemStatus.Enabled {
		if !telegramUsernamePattern.MatchString(c.Telegram.Username) {
			return errors.New("telegram.username must be the bot username without @")
		}
		if !strings.HasSuffix(strings.ToLower(c.Telegram.Username), "bot") {
			return errors.New("telegram.username must end in bot")
		}
	}
	if c.Plugins.AsnkForge.Enabled {
		if err := c.validateAsnkForge(); err != nil {
			return fmt.Errorf("plugins.asnk-forge: %w", err)
		}
	}
	if c.Plugins.MCWhitelist.Enabled {
		if err := c.validateMCWhitelist(); err != nil {
			return fmt.Errorf("plugins.mc-whitelist: %w", err)
		}
	}
	if c.Plugins.SystemStatus.Enabled {
		if err := c.validateSystemStatus(); err != nil {
			return fmt.Errorf("plugins.system-status: %w", err)
		}
	}
	return nil
}

func (c *Config) validateAsnkForge() error {
	p := c.Plugins.AsnkForge
	if strings.TrimSpace(p.ListenAddress) == "" {
		return errors.New("listen-address must not be empty")
	}
	publicURL, err := url.Parse(p.PublicURL)
	if err != nil || publicURL.Host == "" || (publicURL.Scheme != "http" && publicURL.Scheme != "https") {
		return errors.New("public-url must be an absolute HTTP(S) URL")
	}
	if publicURL.User != nil {
		return errors.New("public-url must not contain user information")
	}
	if publicURL.RawQuery != "" || publicURL.Fragment != "" || (publicURL.Path != "" && publicURL.Path != "/") {
		return errors.New("public-url must not contain a path, query, or fragment")
	}
	if publicURL.Scheme != "https" && !isLoopbackHost(publicURL.Hostname()) {
		return errors.New("public-url must use HTTPS except on localhost")
	}
	forgejoURL, err := url.Parse(p.ForgejoURL)
	if err != nil || forgejoURL.Host == "" || (forgejoURL.Scheme != "http" && forgejoURL.Scheme != "https") {
		return errors.New("forgejo-url must be an absolute HTTP(S) URL")
	}
	if forgejoURL.User != nil {
		return errors.New("forgejo-url must not contain user information")
	}
	if forgejoURL.RawQuery != "" || forgejoURL.Fragment != "" {
		return errors.New("forgejo-url must not contain a query or fragment")
	}
	if forgejoURL.Scheme != "https" && !isLoopbackHost(forgejoURL.Hostname()) && !p.AllowInsecureForgejo {
		return errors.New("forgejo-url must use HTTPS unless allow-insecure-forgejo is true")
	}
	if strings.TrimSpace(p.ForgejoAPIToken) == "" {
		return errors.New("forgejo-api-token must not be empty")
	}
	if p.PendingTTL <= 0 || p.SessionTTL <= 0 || p.TelegramAuthMaxAge <= 0 || p.HTTPTimeout <= 0 {
		return errors.New("all durations must be greater than zero")
	}
	return nil
}

func (c *Config) validateMCWhitelist() error {
	p := c.Plugins.MCWhitelist
	if _, _, err := net.SplitHostPort(p.Address); err != nil {
		return fmt.Errorf("address must be host:port: %w", err)
	}
	if p.Password == "" {
		return errors.New("password must not be empty")
	}
	if p.Timeout <= 0 {
		return errors.New("timeout must be greater than zero")
	}
	return nil
}

func (c *Config) validateSystemStatus() error {
	p := c.Plugins.SystemStatus
	if strings.TrimSpace(p.DiskPath) == "" {
		return errors.New("disk-path must not be empty")
	}
	if p.Timeout < 500*time.Millisecond {
		return errors.New("timeout must be at least 500ms")
	}
	return nil
}

func hasDuplicates(values []int64) bool {
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
