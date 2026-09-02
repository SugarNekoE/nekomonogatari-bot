package asnkforge

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"forge.asnk.io/sugar/nekomonogatari-bot/config"
	"forge.asnk.io/sugar/nekomonogatari-bot/data"
	framework "forge.asnk.io/sugar/nekomonogatari-bot/plugin"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const callbackData = "asnk-forge:start"

const registrationReconcileInterval = 30 * time.Second

type implementation struct {
	language config.Language
	telegram config.TelegramConfig
	cfg      config.AsnkForgeConfig
	messages messages
	store    *forgeStore
	forgejo  forgejoClient
	handler  http.Handler
	origin   string
	webURL   string
	now      func() time.Time

	allowedGroups map[int64]struct{}
	botMu         sync.RWMutex
	telegramBot   *bot.Bot
}

func New(ctx context.Context, language config.Language, telegram config.TelegramConfig, cfg config.AsnkForgeConfig, shared *data.Store) (framework.Callbacks, error) {
	p, err := newImplementation(ctx, language, telegram, cfg, shared)
	if err != nil {
		return framework.Callbacks{}, err
	}
	return framework.Callbacks{Register: p.register, Run: p.run}, nil
}

func newImplementation(ctx context.Context, language config.Language, telegram config.TelegramConfig, cfg config.AsnkForgeConfig, shared *data.Store) (*implementation, error) {
	publicURL, err := url.Parse(strings.TrimSpace(cfg.PublicURL))
	if err != nil || publicURL.Scheme == "" || publicURL.Host == "" {
		return nil, errors.New("public URL must be absolute")
	}
	if publicURL.Scheme != "http" && publicURL.Scheme != "https" {
		return nil, errors.New("public URL must use HTTP or HTTPS")
	}
	if cfg.PendingTTL <= 0 || cfg.SessionTTL <= 0 || cfg.TelegramAuthMaxAge <= 0 || cfg.HTTPTimeout <= 0 {
		return nil, errors.New("plugin durations must be positive")
	}
	if strings.TrimSpace(telegram.Token) == "" || strings.TrimSpace(telegram.Username) == "" {
		return nil, errors.New("telegram token and username are required")
	}
	client, err := newHTTPForgejoClient(cfg.ForgejoURL, cfg.ForgejoAPIToken, cfg.HTTPTimeout)
	if err != nil {
		return nil, err
	}
	store, err := newForgeStore(ctx, shared)
	if err != nil {
		return nil, fmt.Errorf("initialize store: %w", err)
	}
	origin := publicURL.Scheme + "://" + publicURL.Host
	webURL := strings.TrimRight(origin, "/") + "/forge/"
	p := &implementation{
		language:      language,
		telegram:      telegram,
		cfg:           cfg,
		messages:      messagesFor(language),
		store:         store,
		forgejo:       client,
		origin:        origin,
		webURL:        webURL,
		now:           time.Now,
		allowedGroups: make(map[int64]struct{}, len(telegram.AllowedGroups)),
	}
	for _, groupID := range telegram.AllowedGroups {
		p.allowedGroups[groupID] = struct{}{}
	}
	p.handler = p.routes()
	return p, nil
}

func (p *implementation) register(b *bot.Bot) error {
	if b == nil {
		return errors.New("telegram bot is required")
	}
	p.botMu.Lock()
	p.telegramBot = b
	p.botMu.Unlock()
	b.RegisterHandlerMatchFunc(framework.Command("forge", p.telegram.Username), p.handleForge)
	b.RegisterHandler(bot.HandlerTypeCallbackQueryData, callbackData, bot.MatchTypeExact, p.handleCallback)
	return nil
}

func (p *implementation) run(ctx context.Context) error {
	if !p.cfg.Enabled {
		return nil
	}
	tracker := newRequestTracker(p.handler)
	server := &http.Server{
		Addr:              p.cfg.ListenAddress,
		Handler:           tracker,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      max(30*time.Second, 2*p.cfg.HTTPTimeout+10*time.Second),
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()
	if err := p.reconcileRegistrations(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("asnk-forge: reconcile registrations: %v", err)
	}
	ticker := time.NewTicker(registrationReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return stopHTTPServer(server, tracker, errCh, p.cfg.HTTPTimeout)
		case err := <-errCh:
			tracker.stop()
			_ = server.Close()
			tracker.wait()
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return fmt.Errorf("serve asnk-forge web: %w", err)
		case <-ticker.C:
			if err := p.reconcileRegistrations(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("asnk-forge: reconcile registrations: %v", err)
			}
		}
	}
}

func stopHTTPServer(server *http.Server, tracker *requestTracker, errCh <-chan error, timeout time.Duration) error {
	tracker.stop()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), max(10*time.Second, 2*timeout+10*time.Second))
	shutdownErr := server.Shutdown(shutdownCtx)
	cancel()
	var closeErr error
	if shutdownErr != nil {
		closeErr = server.Close()
	}
	serveErr := <-errCh
	tracker.wait()
	if errors.Is(serveErr, http.ErrServerClosed) {
		serveErr = nil
	}
	return errors.Join(shutdownErr, closeErr, serveErr)
}

type requestTracker struct {
	next    http.Handler
	mu      sync.Mutex
	active  int
	stopped bool
	done    chan struct{}
}

func newRequestTracker(next http.Handler) *requestTracker {
	return &requestTracker{next: next, done: make(chan struct{})}
}

func (t *requestTracker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	t.mu.Lock()
	if t.stopped {
		t.mu.Unlock()
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		return
	}
	t.active++
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		t.active--
		if t.stopped && t.active == 0 {
			select {
			case <-t.done:
			default:
				close(t.done)
			}
		}
		t.mu.Unlock()
	}()
	t.next.ServeHTTP(w, r)
}

func (t *requestTracker) stop() {
	t.mu.Lock()
	t.stopped = true
	if t.active == 0 {
		select {
		case <-t.done:
		default:
			close(t.done)
		}
	}
	t.mu.Unlock()
}

func (t *requestTracker) wait() {
	<-t.done
}

func (p *implementation) reconcileRegistrations(ctx context.Context) error {
	now := p.now()
	if err := p.store.cleanupExpired(ctx, now); err != nil {
		return err
	}
	if err := p.store.reconcileCompletions(ctx, now); err != nil {
		return err
	}
	if err := p.store.promoteStaleCreating(ctx, now.Add(-p.cfg.HTTPTimeout-time.Minute)); err != nil {
		return err
	}
	registrations, err := p.store.listIndeterminate(ctx)
	if err != nil {
		return err
	}
	var reconciliationErrors []error
	for _, registration := range registrations {
		user, lookupErr := p.lookupIndeterminate(ctx, registration)
		switch {
		case lookupErr == nil:
			if err := p.store.completeRegistration(ctx, registration, user, p.now()); err != nil {
				reconciliationErrors = append(reconciliationErrors, fmt.Errorf("complete registration %d: %w", registration.ID, err))
				continue
			}
			p.notifyRegistration(registration, user)
		case isForgejoNotFound(lookupErr), errors.Is(lookupErr, errForgejoRecoveryMismatch):
			if err := p.store.failRegistration(ctx, registration.ID); err != nil {
				reconciliationErrors = append(reconciliationErrors, fmt.Errorf("release registration %d: %w", registration.ID, err))
			}
		default:
			reconciliationErrors = append(reconciliationErrors, fmt.Errorf("lookup registration %d: %w", registration.ID, lookupErr))
		}
	}
	return errors.Join(reconciliationErrors...)
}

func (p *implementation) allowed(chatID int64) bool {
	_, ok := p.allowedGroups[chatID]
	return ok
}

func (p *implementation) handleForge(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update == nil || update.Message == nil || update.Message.From == nil || !p.allowed(update.Message.Chat.ID) {
		return
	}
	keyboard := &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{
		Text: p.messages.startButton, CallbackData: callbackData,
	}}}}
	sent, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          update.Message.Chat.ID,
		MessageThreadID: update.Message.MessageThreadID,
		Text:            p.messages.commandText,
		ReplyMarkup:     keyboard,
	})
	if err != nil {
		log.Printf("asnk-forge: send command prompt: %v", err)
		return
	}
	if err := p.store.recordPrompt(ctx, sent.Chat.ID, sent.ID, sent.MessageThreadID, p.now().Add(p.cfg.PendingTTL)); err != nil {
		log.Printf("asnk-forge: record command prompt: %v", err)
	}
}

func (p *implementation) handleCallback(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update == nil || update.CallbackQuery == nil {
		return
	}
	query := update.CallbackQuery
	chatID, messageID, threadID, ok := callbackMessage(query)
	if !ok || !p.allowed(chatID) || query.From.IsBot {
		p.answerCallback(ctx, b, query.ID, "", false)
		return
	}
	displayName := framework.DisplayName(&query.From)
	if displayName == "" {
		displayName = fmt.Sprintf("Telegram user %d", query.From.ID)
	}
	err := p.store.claim(ctx, query.ID, query.From.ID, chatID, messageID, threadID,
		query.From.Username, displayName, p.now(), p.cfg.PendingTTL)
	if err != nil {
		switch {
		case errors.Is(err, errAlreadyClaimed):
			p.answerCallback(ctx, b, query.ID, "", false)
		case errors.Is(err, errAlreadyRegistered):
			p.answerCallback(ctx, b, query.ID, p.messages.alreadyRegistered, true)
		case errors.Is(err, errPromptExpired):
			p.answerCallback(ctx, b, query.ID, p.messages.promptExpired, true)
		case errors.Is(err, errRegistrationBusy):
			p.answerCallback(ctx, b, query.ID, p.messages.registrationBusy, true)
		default:
			log.Printf("asnk-forge: claim registration: %v", err)
			p.answerCallback(ctx, b, query.ID, p.messages.internalError, true)
		}
		return
	}
	p.answerCallback(ctx, b, query.ID, "", false)
	text := fmt.Sprintf(p.messages.claimRecorded, displayName, formatTTL(p.cfg.PendingTTL, p.language))
	_, err = b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          chatID,
		MessageThreadID: threadID,
		Text:            text,
		ReplyParameters: &models.ReplyParameters{MessageID: messageID, AllowSendingWithoutReply: true},
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{
			Text: p.messages.openButton, URL: p.webURL,
		}}}},
	})
	if err != nil {
		log.Printf("asnk-forge: send registration URL: %v", err)
	}
}

func (p *implementation) answerCallback(ctx context.Context, b *bot.Bot, queryID, text string, alert bool) {
	_, err := b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: queryID,
		Text:            text,
		ShowAlert:       alert,
	})
	if err != nil {
		log.Printf("asnk-forge: answer callback: %v", err)
	}
}

func callbackMessage(query *models.CallbackQuery) (chatID int64, messageID, threadID int, ok bool) {
	if query == nil {
		return 0, 0, 0, false
	}
	switch query.Message.Type {
	case models.MaybeInaccessibleMessageTypeMessage:
		if query.Message.Message != nil {
			message := query.Message.Message
			return message.Chat.ID, message.ID, message.MessageThreadID, true
		}
	case models.MaybeInaccessibleMessageTypeInaccessibleMessage:
		if query.Message.InaccessibleMessage != nil {
			message := query.Message.InaccessibleMessage
			return message.Chat.ID, message.MessageID, 0, true
		}
	}
	return 0, 0, 0, false
}

func formatTTL(ttl time.Duration, language config.Language) string {
	minutes := int(ttl.Round(time.Minute) / time.Minute)
	if minutes < 1 {
		minutes = 1
	}
	if language == config.LanguageChinese {
		return fmt.Sprintf("%d 分钟", minutes)
	}
	if minutes == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", minutes)
}
