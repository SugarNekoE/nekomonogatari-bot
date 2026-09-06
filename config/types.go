package config

import "time"

type Language string

const (
	LanguageEnglish Language = "en"
	LanguageChinese Language = "zh"
)

type Config struct {
	Language Language       `mapstructure:"language" yaml:"language"`
	Telegram TelegramConfig `mapstructure:"telegram" yaml:"telegram"`
	Database DatabaseConfig `mapstructure:"database" yaml:"database"`
	Plugins  PluginsConfig  `mapstructure:"plugins" yaml:"plugins"`
}

type TelegramConfig struct {
	Token         string  `mapstructure:"token" yaml:"token"`
	Username      string  `mapstructure:"username" yaml:"username"`
	AllowedGroups []int64 `mapstructure:"allowed-groups" yaml:"allowed-groups"`
}

type DatabaseConfig struct {
	Path string `mapstructure:"path" yaml:"path"`
}

type PluginsConfig struct {
	AsnkForge    AsnkForgeConfig    `mapstructure:"asnk-forge" yaml:"asnk-forge"`
	MCWhitelist  MCWhitelistConfig  `mapstructure:"mc-whitelist" yaml:"mc-whitelist"`
	MCStatus     MCStatusConfig     `mapstructure:"mc-status" yaml:"mc-status"`
	SystemStatus SystemStatusConfig `mapstructure:"system-status" yaml:"system-status"`
}

type AsnkForgeConfig struct {
	Enabled              bool          `mapstructure:"enabled" yaml:"enabled"`
	ListenAddress        string        `mapstructure:"listen-address" yaml:"listen-address"`
	PublicURL            string        `mapstructure:"public-url" yaml:"public-url"`
	ForgejoURL           string        `mapstructure:"forgejo-url" yaml:"forgejo-url"`
	ForgejoAPIToken      string        `mapstructure:"forgejo-api-token" yaml:"forgejo-api-token"`
	AllowInsecureForgejo bool          `mapstructure:"allow-insecure-forgejo" yaml:"allow-insecure-forgejo"`
	PendingTTL           time.Duration `mapstructure:"pending-ttl" yaml:"pending-ttl"`
	SessionTTL           time.Duration `mapstructure:"session-ttl" yaml:"session-ttl"`
	TelegramAuthMaxAge   time.Duration `mapstructure:"telegram-auth-max-age" yaml:"telegram-auth-max-age"`
	HTTPTimeout          time.Duration `mapstructure:"http-timeout" yaml:"http-timeout"`
}

type MCWhitelistConfig struct {
	Enabled  bool          `mapstructure:"enabled" yaml:"enabled"`
	Address  string        `mapstructure:"address" yaml:"address"`
	Password string        `mapstructure:"password" yaml:"password"`
	Timeout  time.Duration `mapstructure:"timeout" yaml:"timeout"`
}

type SystemStatusConfig struct {
	Enabled      bool          `mapstructure:"enabled" yaml:"enabled"`
	DiskPath     string        `mapstructure:"disk-path" yaml:"disk-path"`
	ShowHostname bool          `mapstructure:"show-hostname" yaml:"show-hostname"`
	Timeout      time.Duration `mapstructure:"timeout" yaml:"timeout"`
}

type MCStatusConfig struct {
	Enabled bool          `mapstructure:"enabled" yaml:"enabled"`
	Name    string        `mapstructure:"name" yaml:"name"`
	Address string        `mapstructure:"address" yaml:"address"`
	Timeout time.Duration `mapstructure:"timeout" yaml:"timeout"`
}
