package mcstatus

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"forge.asnk.io/sugar/nekomonogatari-bot/config"
	framework "forge.asnk.io/sugar/nekomonogatari-bot/plugin"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type plugin struct {
	enabled     bool
	language    config.Language
	botUsername string
	name        string
	address     string
	timeout     time.Duration
	query       func(context.Context, string) (status, error)
}

func New(language config.Language, telegram config.TelegramConfig, cfg config.MCStatusConfig) (framework.Callbacks, error) {
	if language != config.LanguageEnglish && language != config.LanguageChinese {
		return framework.Callbacks{}, fmt.Errorf("mc-status: unsupported language %q", language)
	}
	p := &plugin{
		enabled: cfg.Enabled, language: language,
		botUsername: strings.TrimPrefix(strings.TrimSpace(telegram.Username), "@"),
		name:        strings.TrimSpace(cfg.Name), address: strings.TrimSpace(cfg.Address), timeout: cfg.Timeout,
	}
	if cfg.Enabled {
		if p.botUsername == "" {
			return framework.Callbacks{}, errors.New("mc-status: Telegram bot username must not be empty")
		}
		if err := cfg.Validate(); err != nil {
			return framework.Callbacks{}, fmt.Errorf("mc-status: %w", err)
		}
		if _, err := parseAddress(p.address); err != nil {
			return framework.Callbacks{}, fmt.Errorf("mc-status: address: %w", err)
		}
		client := statusClient{lookupSRV: net.DefaultResolver.LookupSRV, dial: (&net.Dialer{}).DialContext}
		p.query = client.query
	}
	return framework.Callbacks{Register: p.register}, nil
}

func (p *plugin) register(b *bot.Bot) error {
	if !p.enabled {
		return nil
	}
	if b == nil {
		return errors.New("telegram bot must not be nil")
	}
	b.RegisterHandlerMatchFunc(framework.Command("mc", p.botUsername), p.handleCommand)
	return nil
}

func (p *plugin) handleCommand(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update == nil || update.Message == nil || update.Message.From == nil || update.Message.From.IsBot {
		return
	}
	message := update.Message
	_, args := framework.ParseCommand(message.Text)
	labels := catalog[p.language]
	text := labels.usage
	if len(args) <= 1 {
		name, address := p.name, p.address
		if len(args) == 1 {
			address = args[0]
			name = address
		}
		if _, err := parseAddress(address); err == nil {
			queryCtx, cancel := context.WithTimeout(ctx, p.timeout)
			result, err := p.query(queryCtx, address)
			cancel()
			if err != nil {
				log.Printf("[mc-status] query %q: %v", address, err)
				text = fmt.Sprintf("%s\n%s: %s\n%s", plainText(name, 200), labels.address, address, labels.unavailable)
			} else {
				text = formatStatus(p.language, name, address, result)
			}
		}
	}
	if _, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: message.Chat.ID, MessageThreadID: message.MessageThreadID, Text: text,
		LinkPreviewOptions: &models.LinkPreviewOptions{IsDisabled: bot.True()},
		ReplyParameters:    &models.ReplyParameters{MessageID: message.ID, AllowSendingWithoutReply: true},
	}); err != nil {
		log.Printf("[mc-status] send Telegram message: %v", err)
	}
}
