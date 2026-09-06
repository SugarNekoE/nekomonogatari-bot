package mcstatus

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"forge.asnk.io/sugar/nekomonogatari-bot/config"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type telegramClient struct {
	values map[string][]string
	calls  int
}

func (c *telegramClient) Do(r *http.Request) (*http.Response, error) {
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		return nil, err
	}
	c.values = r.MultipartForm.Value
	c.calls++
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true,"result":{"message_id":77,"date":1,"chat":{"id":-1001,"type":"supergroup"}}}`))}, nil
}

func TestCommand(t *testing.T) {
	for _, test := range []struct {
		name, command, address, want string
		language                     config.Language
		queryErr                     error
		isBot, ignored               bool
	}{
		{name: "configured", command: "/mc", address: "play.test", want: "Our Server\nServer address: play.test\nPlayers: 3/20\nMOTD:\nHello\n<world>", language: config.LanguageEnglish},
		{name: "override", command: "/mc other.test:25566", address: "other.test:25566", want: "other.test:25566\nServer address: other.test:25566", language: config.LanguageEnglish},
		{name: "mention and Chinese", command: "/mc@NekoBot", address: "play.test", want: "在线人数: 3/20", language: config.LanguageChinese},
		{name: "offline", command: "/mc", address: "play.test", want: "Server status unavailable", language: config.LanguageEnglish, queryErr: errors.New("offline")},
		{name: "invalid address", command: "/mc https://bad.test", want: "Usage:", language: config.LanguageEnglish},
		{name: "extra args", command: "/mc one.test two.test", want: "Usage:", language: config.LanguageEnglish},
		{name: "other bot", command: "/mc@OtherBot", ignored: true, language: config.LanguageEnglish},
		{name: "bot sender", command: "/mc", isBot: true, ignored: true, language: config.LanguageEnglish},
	} {
		t.Run(test.name, func(t *testing.T) {
			queries := 0
			p := &plugin{enabled: true, language: test.language, botUsername: "NekoBot", name: "Our Server", address: "play.test", timeout: time.Second,
				query: func(ctx context.Context, address string) (status, error) {
					queries++
					if address != test.address {
						t.Errorf("queried %q, want %q", address, test.address)
					}
					if _, ok := ctx.Deadline(); !ok {
						t.Error("query has no deadline")
					}
					var result status
					if err := json.Unmarshal([]byte(`{"description":["§aHello",{"text":"\n<world>"}],"players":{"online":3,"max":20}}`), &result); err != nil {
						t.Fatal(err)
					}
					return result, test.queryErr
				},
			}
			client := &telegramClient{}
			b, err := bot.New("test-token", bot.WithSkipGetMe(), bot.WithNotAsyncHandlers(), bot.WithHTTPClient(time.Second, client), bot.WithDefaultHandler(func(context.Context, *bot.Bot, *models.Update) {}))
			if err != nil {
				t.Fatal(err)
			}
			if err := p.register(b); err != nil {
				t.Fatal(err)
			}
			b.ProcessUpdate(context.Background(), &models.Update{Message: &models.Message{
				ID: 10, MessageThreadID: 3, Text: test.command, From: &models.User{ID: 101, IsBot: test.isBot}, Chat: models.Chat{ID: -1001, Type: models.ChatTypeSupergroup},
			}})
			if test.ignored {
				if client.calls != 0 || queries != 0 {
					t.Fatal("ignored command caused activity")
				}
				return
			}
			if client.calls != 1 {
				t.Fatalf("Telegram calls = %d", client.calls)
			}
			text := client.values["text"][0]
			if !strings.Contains(text, test.want) {
				t.Errorf("text = %q, want %q", text, test.want)
			}
			if test.address != "" && test.queryErr == nil && !strings.Contains(text, "MOTD:\nHello\n<world>") {
				t.Errorf("missing MOTD: %q", text)
			}
			if (queries == 1) != (test.address != "") {
				t.Errorf("queries = %d", queries)
			}
			if client.values["message_thread_id"][0] != "3" || !strings.Contains(client.values["reply_parameters"][0], `"message_id":10`) {
				t.Errorf("reply metadata = %#v", client.values)
			}
			if len(client.values["parse_mode"]) != 0 {
				t.Error("server text should be plain text")
			}
		})
	}
}

func TestNew(t *testing.T) {
	disabled, err := New(config.LanguageEnglish, config.TelegramConfig{}, config.MCStatusConfig{})
	if err != nil || disabled.Register == nil || disabled.Run != nil {
		t.Fatalf("disabled callbacks: %#v, %v", disabled, err)
	}
	if err := disabled.Register(nil); err != nil {
		t.Fatal(err)
	}
	valid := config.MCStatusConfig{Enabled: true, Name: "Server", Address: "play.test", Timeout: time.Second}
	callbacks, err := New(config.LanguageEnglish, config.TelegramConfig{Username: "NekoBot"}, valid)
	if err != nil {
		t.Fatal(err)
	}
	if err := callbacks.Register(nil); err == nil {
		t.Fatal("accepted nil bot")
	}
	for _, change := range []func(*config.MCStatusConfig){
		func(c *config.MCStatusConfig) { c.Name = "" },
		func(c *config.MCStatusConfig) { c.Address = "https://bad.test" },
		func(c *config.MCStatusConfig) { c.Timeout = 0 },
	} {
		cfg := valid
		change(&cfg)
		if _, err := New(config.LanguageEnglish, config.TelegramConfig{Username: "NekoBot"}, cfg); err == nil {
			t.Fatalf("accepted %#v", cfg)
		}
	}
	if _, err := New("jp", config.TelegramConfig{Username: "NekoBot"}, valid); err == nil {
		t.Fatal("accepted language")
	}
	if _, err := New(config.LanguageEnglish, config.TelegramConfig{}, valid); err == nil {
		t.Fatal("accepted empty username")
	}
}

func TestMOTDFormatsAndMessageLength(t *testing.T) {
	for _, test := range []struct{ raw, want string }{
		{`"§x§F§F§0§0§0§0Hello"`, "Hello"},
		{`{"text":"Hello","extra":[{"text":"\n","extra":["world"]}]}`, "Hello\nworld"},
		{`["Hello",{"text":" world"}]`, "Hello world"},
		{`{"translate":"server.welcome","fallback":"Welcome"}`, "Welcome"},
		{`null`, "(empty)"},
	} {
		text := formatStatus(config.LanguageEnglish, "Server", "play.test", status{Description: json.RawMessage(test.raw)})
		if !strings.Contains(text, "MOTD:\n"+test.want) || !strings.Contains(text, "Players: Unknown") {
			t.Errorf("message = %q", text)
		}
	}
	raw, _ := json.Marshal(strings.Repeat("😀", 10000))
	text := formatStatus(config.LanguageChinese, strings.Repeat("😀", 1000), strings.Repeat("a", 253)+":65535", status{Description: raw})
	if len(utf16.Encode([]rune(text))) > 4096 || !strings.Contains(text, "…") {
		t.Fatal("message exceeds Telegram limit or lacks truncation marker")
	}
}
