package asnkforge

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

type forgeTelegramCall struct {
	method string
	values map[string][]string
}

type forgeTelegramClient struct {
	mu    sync.Mutex
	calls []forgeTelegramCall
}

func (f *forgeTelegramClient) Do(request *http.Request) (*http.Response, error) {
	if err := request.ParseMultipartForm(1 << 20); err != nil {
		return nil, err
	}
	values := make(map[string][]string, len(request.MultipartForm.Value))
	for key, value := range request.MultipartForm.Value {
		values[key] = append([]string(nil), value...)
	}
	method := path.Base(request.URL.Path)
	f.mu.Lock()
	f.calls = append(f.calls, forgeTelegramCall{method: method, values: values})
	f.mu.Unlock()
	result := "true"
	if method == "sendMessage" {
		result = `{"message_id":77,"message_thread_id":4,"date":1,"chat":{"id":-100123,"type":"supergroup"},"text":"ok"}`
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"ok":true,"result":` + result + `}`)),
	}, nil
}

func (f *forgeTelegramClient) takeCalls() []forgeTelegramCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	calls := append([]forgeTelegramCall(nil), f.calls...)
	f.calls = nil
	return calls
}

func TestForgeCommandClaimAndConstantLink(t *testing.T) {
	p, _, now := testImplementation(t, config.LanguageEnglish)
	client := &forgeTelegramClient{}
	b, err := bot.New(
		"123456:test-bot-token",
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
		ID:              12,
		MessageThreadID: 4,
		Text:            "/forge@NekoForgeBot",
		From:            &models.User{ID: 777, Username: "neko"},
		Chat:            models.Chat{ID: -100123, Type: models.ChatTypeSupergroup},
	}})
	calls := client.takeCalls()
	if len(calls) != 1 || calls[0].method != "sendMessage" {
		t.Fatalf("command calls = %#v", calls)
	}
	if !strings.Contains(calls[0].values["reply_markup"][0], callbackData) {
		t.Fatalf("prompt markup = %#v", calls[0].values)
	}

	b.ProcessUpdate(context.Background(), &models.Update{CallbackQuery: &models.CallbackQuery{
		ID:   "callback-777",
		From: models.User{ID: 777, Username: "neko"},
		Data: callbackData,
		Message: models.MaybeInaccessibleMessage{
			Type: models.MaybeInaccessibleMessageTypeMessage,
			Message: &models.Message{
				ID:              77,
				MessageThreadID: 4,
				Date:            1,
				Chat:            models.Chat{ID: -100123, Type: models.ChatTypeSupergroup},
			},
		},
	}})
	calls = client.takeCalls()
	if len(calls) != 2 || calls[0].method != "answerCallbackQuery" || calls[1].method != "sendMessage" {
		t.Fatalf("callback calls = %#v", calls)
	}
	markup := calls[1].values["reply_markup"][0]
	if !strings.Contains(markup, `"url":"https://register.example/forge/"`) {
		t.Fatalf("registration markup = %s", markup)
	}
	for _, forbidden := range []string{"callback-777", "777", "token", "nonce"} {
		if strings.Contains(p.webURL, forbidden) {
			t.Fatalf("web URL %q contains %q", p.webURL, forbidden)
		}
	}

	if _, err := p.store.authenticate(context.Background(), 777, digestToken("session"), digestToken("csrf"), now, p.cfg.SessionTTL); err != nil {
		t.Fatalf("claimed Telegram account could not authenticate: %v", err)
	}
	if _, err := p.store.authenticate(context.Background(), 778, digestToken("other"), digestToken("csrf"), now, p.cfg.SessionTTL); !errors.Is(err, errNoPending) {
		t.Fatalf("different Telegram account error = %v", err)
	}
}
