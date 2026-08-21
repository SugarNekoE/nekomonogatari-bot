package plugin

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type roundTripClient func(*http.Request) (*http.Response, error)

func (f roundTripClient) Do(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestParseCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		text    string
		command string
		args    []string
	}{
		{"/forge", "forge", nil},
		{" /mcwl@NekoBot ADD Steve ", "mcwl", []string{"ADD", "Steve"}},
		{"hello", "", nil},
		{"", "", nil},
	}
	for _, test := range tests {
		command, args := ParseCommand(test.text)
		if command != test.command {
			t.Errorf("ParseCommand(%q) command = %q, want %q", test.text, command, test.command)
		}
		if len(args) != len(test.args) {
			t.Errorf("ParseCommand(%q) args = %v, want %v", test.text, args, test.args)
		}
	}
}

func TestCommandChecksBotSuffix(t *testing.T) {
	t.Parallel()
	match := Command("mcwl", "NekoBot")
	for text, want := range map[string]bool{
		"/mcwl list":            true,
		"/mcwl@NekoBot list":    true,
		"/mcwl@nekobot list":    true,
		"/mcwl@AnotherBot list": false,
	} {
		got := match(&models.Update{Message: &models.Message{Text: text}})
		if got != want {
			t.Errorf("Command match for %q = %v, want %v", text, got, want)
		}
	}
}

func TestChatID(t *testing.T) {
	t.Parallel()
	messageUpdate := &models.Update{Message: &models.Message{Chat: models.Chat{ID: -1001}}}
	if got, ok := ChatID(messageUpdate); !ok || got != -1001 {
		t.Fatalf("ChatID(message) = %d, %v", got, ok)
	}

	callbackUpdate := &models.Update{CallbackQuery: &models.CallbackQuery{
		Message: models.MaybeInaccessibleMessage{
			Type:    models.MaybeInaccessibleMessageTypeMessage,
			Message: &models.Message{Chat: models.Chat{ID: -1002}},
		},
	}}
	if got, ok := ChatID(callbackUpdate); !ok || got != -1002 {
		t.Fatalf("ChatID(callback) = %d, %v", got, ok)
	}
}

func TestAllowGroups(t *testing.T) {
	t.Parallel()

	var apiCalls atomic.Int32
	b, err := bot.New(
		"123456:test-token",
		bot.WithSkipGetMe(),
		bot.WithNotAsyncHandlers(),
		bot.WithHTTPClient(0, roundTripClient(func(*http.Request) (*http.Response, error) {
			apiCalls.Add(1)
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"ok":true,"result":true}`)),
				Header:     make(http.Header),
			}, nil
		})),
	)
	if err != nil {
		t.Fatal(err)
	}

	var handled atomic.Int32
	middleware := AllowGroups([]int64{-1001})
	handler := middleware(func(context.Context, *bot.Bot, *models.Update) {
		handled.Add(1)
	})

	handler(context.Background(), b, &models.Update{Message: &models.Message{Chat: models.Chat{ID: -1001, Type: models.ChatTypeSupergroup}}})
	handler(context.Background(), b, &models.Update{Message: &models.Message{Chat: models.Chat{ID: -2002, Type: models.ChatTypeGroup}}})
	handler(context.Background(), b, &models.Update{Message: &models.Message{Chat: models.Chat{ID: -1001, Type: models.ChatTypePrivate}}})
	handler(context.Background(), b, &models.Update{CallbackQuery: &models.CallbackQuery{
		ID: "denied",
		Message: models.MaybeInaccessibleMessage{
			Type:    models.MaybeInaccessibleMessageTypeMessage,
			Message: &models.Message{Chat: models.Chat{ID: -2002, Type: models.ChatTypeSupergroup}},
		},
	}})

	if got := handled.Load(); got != 1 {
		t.Errorf("handled = %d, want 1", got)
	}
	if got := apiCalls.Load(); got != 1 {
		t.Errorf("API calls = %d, want 1 callback answer", got)
	}
}
