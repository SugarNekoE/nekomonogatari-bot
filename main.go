package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"forge.asnk.io/sugar/nekomonogatari-bot/config"
	"forge.asnk.io/sugar/nekomonogatari-bot/data"
	"forge.asnk.io/sugar/nekomonogatari-bot/plugin"
	asnkforge "forge.asnk.io/sugar/nekomonogatari-bot/plugin/asnk-forge"
	mcwhitelist "forge.asnk.io/sugar/nekomonogatari-bot/plugin/mc-whitelist"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("nekomonogatari-bot: %v", err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sharedData, err := data.Open(ctx, cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("open data store: %w", err)
	}
	defer sharedData.Close()

	b, err := bot.New(
		cfg.Telegram.Token,
		bot.WithDefaultHandler(func(context.Context, *bot.Bot, *models.Update) {}),
		bot.WithMiddlewares(plugin.AllowGroups(cfg.Telegram.AllowedGroups)),
		bot.WithNotAsyncHandlers(),
		bot.WithWorkers(4),
		bot.WithErrorsHandler(func(err error) {
			log.Printf("telegram error: %v", err)
		}),
	)
	if err != nil {
		return fmt.Errorf("create Telegram bot: %w", err)
	}

	registry := plugin.NewRegistry()
	if cfg.Plugins.AsnkForge.Enabled {
		forgeCallbacks, err := asnkforge.New(ctx, cfg.Language, cfg.Telegram, cfg.Plugins.AsnkForge, sharedData)
		if err != nil {
			return fmt.Errorf("initialize asnk-forge: %w", err)
		}
		if err := registry.Add("asnk-forge", forgeCallbacks); err != nil {
			return err
		}
	}
	if cfg.Plugins.MCWhitelist.Enabled {
		minecraftCallbacks, err := mcwhitelist.New(ctx, cfg.Language, cfg.Telegram, cfg.Plugins.MCWhitelist, sharedData)
		if err != nil {
			return fmt.Errorf("initialize mc-whitelist: %w", err)
		}
		if err := registry.Add("mc-whitelist", minecraftCallbacks); err != nil {
			return err
		}
	}
	if err := registry.RegisterHandlers(b); err != nil {
		return err
	}

	registry.Start(ctx)
	backgroundErr := make(chan error, 1)
	go func() {
		select {
		case err := <-registry.Errors():
			backgroundErr <- err
			stop()
		case <-ctx.Done():
		}
	}()

	names := registry.Names()
	if len(names) == 0 {
		log.Printf("starting Telegram bot with no enabled plugins")
	} else {
		log.Printf("starting Telegram bot with plugins: %s", strings.Join(names, ", "))
	}
	b.Start(ctx)
	registry.Wait()

	select {
	case err := <-backgroundErr:
		return err
	default:
		return nil
	}
}
