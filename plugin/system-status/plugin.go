package systemstatus

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"forge.asnk.io/sugar/nekomonogatari-bot/config"
	framework "forge.asnk.io/sugar/nekomonogatari-bot/plugin"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const pluginName = "system-status"

type plugin struct {
	enabled     bool
	language    config.Language
	botUsername string
	timeout     time.Duration
	collector   collector
}

func New(language config.Language, telegram config.TelegramConfig, cfg config.SystemStatusConfig) (framework.Callbacks, error) {
	return newPlugin(language, telegram, cfg, nil)
}

func newPlugin(language config.Language, telegram config.TelegramConfig, cfg config.SystemStatusConfig, statusCollector collector) (framework.Callbacks, error) {
	if language != config.LanguageEnglish && language != config.LanguageChinese {
		return framework.Callbacks{}, fmt.Errorf("%s: unsupported language %q", pluginName, language)
	}
	p := &plugin{
		enabled:     cfg.Enabled,
		language:    language,
		botUsername: strings.TrimPrefix(strings.TrimSpace(telegram.Username), "@"),
		timeout:     cfg.Timeout,
	}
	if !cfg.Enabled {
		return framework.Callbacks{Register: p.register}, nil
	}
	if p.botUsername == "" {
		return framework.Callbacks{}, fmt.Errorf("%s: Telegram bot username must not be empty", pluginName)
	}
	if strings.TrimSpace(cfg.DiskPath) == "" {
		return framework.Callbacks{}, fmt.Errorf("%s: disk path must not be empty", pluginName)
	}
	if cfg.Timeout < 500*time.Millisecond {
		return framework.Callbacks{}, fmt.Errorf("%s: timeout must be at least 500ms", pluginName)
	}
	if statusCollector == nil {
		statusCollector = &systemCollector{
			diskPath:     strings.TrimSpace(cfg.DiskPath),
			showHostname: cfg.ShowHostname,
			startedAt:    time.Now(),
		}
	}
	p.collector = statusCollector
	return framework.Callbacks{Register: p.register}, nil
}

func (p *plugin) register(b *bot.Bot) error {
	if !p.enabled {
		return nil
	}
	if b == nil {
		return errors.New("telegram bot must not be nil")
	}
	b.RegisterHandlerMatchFunc(framework.Command("status", p.botUsername), p.handleStatus)
	return nil
}

func (p *plugin) handleStatus(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update == nil || update.Message == nil || update.Message.From == nil || update.Message.From.IsBot {
		return
	}
	collectionContext, cancel := context.WithTimeout(ctx, p.timeout)
	snapshot, err := p.collector.Collect(collectionContext)
	cancel()
	if err != nil {
		log.Printf("[%s] collect metrics: %v", pluginName, err)
	}
	text := statusCatalog[p.language].unavailable
	if snapshot.usable() {
		text = formatStatus(p.language, snapshot)
	}
	message := update.Message
	if _, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          message.Chat.ID,
		MessageThreadID: message.MessageThreadID,
		Text:            text,
		ReplyParameters: &models.ReplyParameters{
			MessageID:                message.ID,
			AllowSendingWithoutReply: true,
		},
	}); err != nil {
		log.Printf("[%s] send Telegram message: %v", pluginName, err)
	}
}
