package mcwhitelist

import (
	"context"
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

type telegramCall struct {
	method string
	values map[string][]string
}

type fakeTelegramClient struct {
	mu    sync.Mutex
	calls []telegramCall
}

func (f *fakeTelegramClient) Do(request *http.Request) (*http.Response, error) {
	if err := request.ParseMultipartForm(1 << 20); err != nil {
		return nil, err
	}
	values := make(map[string][]string, len(request.MultipartForm.Value))
	for key, value := range request.MultipartForm.Value {
		values[key] = append([]string(nil), value...)
	}
	method := path.Base(request.URL.Path)
	f.mu.Lock()
	f.calls = append(f.calls, telegramCall{method: method, values: values})
	f.mu.Unlock()
	result := "true"
	if method == "sendMessage" || method == "editMessageText" {
		result = `{"message_id":900,"date":1,"chat":{"id":-1001,"type":"supergroup"},"text":"ok"}`
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"ok":true,"result":` + result + `}`)),
	}, nil
}

func (f *fakeTelegramClient) takeCalls() []telegramCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	calls := append([]telegramCall(nil), f.calls...)
	f.calls = nil
	return calls
}

func newHandlerTestBot(t *testing.T, client *fakeTelegramClient) *bot.Bot {
	t.Helper()
	b, err := bot.New("test-token",
		bot.WithSkipGetMe(),
		bot.WithHTTPClient(time.Second, client),
		bot.WithNotAsyncHandlers(),
	)
	if err != nil {
		t.Fatalf("new bot: %v", err)
	}
	return b
}

func TestBareCommandMenuAndOwnerBoundCallbacks(t *testing.T) {
	store := openTestPlayerStore(t)
	p := &plugin{
		enabled:     true,
		language:    config.LanguageEnglish,
		botUsername: "NekoBot",
		service:     &whitelistService{store: store, remote: &fakeWhitelister{}},
	}
	client := &fakeTelegramClient{}
	b := newHandlerTestBot(t, client)
	if err := p.register(b); err != nil {
		t.Fatal(err)
	}
	b.ProcessUpdate(context.Background(), &models.Update{Message: &models.Message{
		ID:   10,
		Text: "/mcwl@NekoBot",
		From: &models.User{ID: 101, FirstName: "Owner"},
		Chat: models.Chat{ID: -1001, Type: models.ChatTypeSupergroup},
	}})
	calls := client.takeCalls()
	if len(calls) != 1 || calls[0].method != "sendMessage" {
		t.Fatalf("bare command calls = %#v", calls)
	}
	if !strings.Contains(calls[0].values["text"][0], "Minecraft whitelist") ||
		!strings.Contains(calls[0].values["reply_markup"][0], "mcwl|101|add") {
		t.Fatalf("bare command payload = %#v", calls[0].values)
	}

	callbackMessage := models.MaybeInaccessibleMessage{
		Type: models.MaybeInaccessibleMessageTypeMessage,
		Message: &models.Message{
			ID:   900,
			Date: 1,
			Chat: models.Chat{ID: -1001, Type: models.ChatTypeSupergroup},
		},
	}
	b.ProcessUpdate(context.Background(), &models.Update{CallbackQuery: &models.CallbackQuery{
		ID:      "wrong-owner",
		From:    models.User{ID: 202},
		Data:    "mcwl|101|list",
		Message: callbackMessage,
	}})
	calls = client.takeCalls()
	if len(calls) != 1 || calls[0].method != "answerCallbackQuery" || calls[0].values["show_alert"][0] != "true" {
		t.Fatalf("wrong-owner calls = %#v", calls)
	}
	if !strings.Contains(calls[0].values["text"][0], "another user") {
		t.Fatalf("wrong-owner alert = %#v", calls[0].values)
	}

	b.ProcessUpdate(context.Background(), &models.Update{CallbackQuery: &models.CallbackQuery{
		ID:      "owner-list",
		From:    models.User{ID: 101},
		Data:    "mcwl|101|list",
		Message: callbackMessage,
	}})
	calls = client.takeCalls()
	if len(calls) != 2 || calls[0].method != "answerCallbackQuery" || calls[1].method != "editMessageText" {
		t.Fatalf("owner-list calls = %#v", calls)
	}
	if !strings.Contains(calls[1].values["text"][0], "no bound Minecraft players") {
		t.Fatalf("owner-list text = %#v", calls[1].values)
	}
}

func TestInvalidAddReturnsChineseInstructionWithoutRCON(t *testing.T) {
	store := openTestPlayerStore(t)
	remote := &fakeWhitelister{}
	p := &plugin{
		enabled:     true,
		language:    config.LanguageChinese,
		botUsername: "NekoBot",
		service:     &whitelistService{store: store, remote: remote},
	}
	client := &fakeTelegramClient{}
	b := newHandlerTestBot(t, client)
	if err := p.register(b); err != nil {
		t.Fatal(err)
	}
	b.ProcessUpdate(context.Background(), &models.Update{Message: &models.Message{
		ID:   11,
		Text: "/mcwl add bad;name",
		From: &models.User{ID: 101},
		Chat: models.Chat{ID: -1001, Type: models.ChatTypeSupergroup},
	}})
	calls := client.takeCalls()
	if len(calls) != 1 || calls[0].method != "sendMessage" {
		t.Fatalf("invalid-add calls = %#v", calls)
	}
	if !strings.Contains(calls[0].values["text"][0], "玩家名格式无效") {
		t.Fatalf("invalid-add text = %#v", calls[0].values)
	}
	if len(remote.addCalls) != 0 {
		t.Fatalf("invalid name reached RCON: %v", remote.addCalls)
	}
}
