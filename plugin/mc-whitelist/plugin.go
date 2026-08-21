package mcwhitelist

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"forge.asnk.io/sugar/nekomonogatari-bot/config"
	"forge.asnk.io/sugar/nekomonogatari-bot/data"
	framework "forge.asnk.io/sugar/nekomonogatari-bot/plugin"
	"github.com/go-telegram/bot"
)

const (
	pluginName        = "mc-whitelist"
	callbackPrefix    = "mcwl|"
	reconcileInterval = 30 * time.Second
)

type plugin struct {
	enabled     bool
	language    config.Language
	botUsername string
	service     *whitelistService
}

func New(ctx context.Context, language config.Language, telegram config.TelegramConfig, cfg config.MCWhitelistConfig, shared *data.Store) (framework.Callbacks, error) {
	if language != config.LanguageEnglish && language != config.LanguageChinese {
		return framework.Callbacks{}, fmt.Errorf("%s: unsupported language %q", pluginName, language)
	}
	botUsername := strings.TrimPrefix(strings.TrimSpace(telegram.Username), "@")
	p := &plugin{enabled: cfg.Enabled, language: language, botUsername: botUsername}
	if !cfg.Enabled {
		return framework.Callbacks{Register: p.register}, nil
	}
	if botUsername == "" {
		return framework.Callbacks{}, fmt.Errorf("%s: Telegram bot username must not be empty", pluginName)
	}
	if shared == nil {
		return framework.Callbacks{}, fmt.Errorf("%s: shared data store must not be nil", pluginName)
	}
	address := strings.TrimSpace(cfg.Address)
	if _, _, err := net.SplitHostPort(address); err != nil {
		return framework.Callbacks{}, fmt.Errorf("%s: RCON address must be host:port: %w", pluginName, err)
	}
	if cfg.Password == "" {
		return framework.Callbacks{}, fmt.Errorf("%s: RCON password must not be empty", pluginName)
	}
	if strings.IndexByte(cfg.Password, 0) >= 0 {
		return framework.Callbacks{}, fmt.Errorf("%s: RCON password must not contain NUL", pluginName)
	}
	if len(cfg.Password) > rconMaxPacketSize-10 {
		return framework.Callbacks{}, fmt.Errorf("%s: RCON password is too long", pluginName)
	}
	if cfg.Timeout <= 0 {
		return framework.Callbacks{}, fmt.Errorf("%s: RCON timeout must be greater than zero", pluginName)
	}

	store, err := newPlayerStore(ctx, shared)
	if err != nil {
		return framework.Callbacks{}, fmt.Errorf("initialize %s data: %w", pluginName, err)
	}
	remote := &minecraftWhitelister{executor: &rconExecutor{
		address:  address,
		password: cfg.Password,
		timeout:  cfg.Timeout,
	}}
	p.service = &whitelistService{store: store, remote: remote}
	return framework.Callbacks{Register: p.register, Run: p.run}, nil
}

func (p *plugin) register(b *bot.Bot) error {
	if !p.enabled {
		return nil
	}
	if b == nil {
		return errors.New("telegram bot must not be nil")
	}
	b.RegisterHandlerMatchFunc(framework.Command("mcwl", p.botUsername), p.handleCommand)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, callbackPrefix, bot.MatchTypePrefix, p.handleCallback)
	return nil
}

func (p *plugin) run(ctx context.Context) error {
	if !p.enabled {
		return nil
	}
	if err := p.service.reconcile(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("[%s] initial reconciliation: %v", pluginName, err)
	}
	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := p.service.reconcile(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("[%s] reconciliation: %v", pluginName, err)
			}
		}
	}
}
