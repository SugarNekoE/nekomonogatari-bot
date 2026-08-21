package asnkforge

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"forge.asnk.io/sugar/nekomonogatari-bot/config"
	"forge.asnk.io/sugar/nekomonogatari-bot/data"
)

const testBotToken = "123456:test-bot-token"

func signedTelegramAuth(userID int64, now time.Time) telegramAuthData {
	data := telegramAuthData{
		ID:        fmt.Sprintf("%d", userID),
		FirstName: "Neko",
		LastName:  "Tester",
		Username:  "neko_test",
		AuthDate:  fmt.Sprintf("%d", now.Unix()),
	}
	fields := map[string]string{
		"auth_date":  data.AuthDate,
		"first_name": data.FirstName,
		"id":         data.ID,
		"last_name":  data.LastName,
		"username":   data.Username,
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+fields[key])
	}
	secret := sha256.Sum256([]byte(testBotToken))
	mac := hmac.New(sha256.New, secret[:])
	_, _ = mac.Write([]byte(strings.Join(parts, "\n")))
	data.Hash = hex.EncodeToString(mac.Sum(nil))
	return data
}

func testImplementation(t *testing.T, language config.Language) (*implementation, *data.Store, time.Time) {
	t.Helper()
	ctx := context.Background()
	shared, err := data.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("data.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = shared.Close() })
	now := time.Unix(1_900_000_000, 0)
	cfg := config.AsnkForgeConfig{
		Enabled: true, ListenAddress: "127.0.0.1:0", PublicURL: "https://register.example",
		ForgejoURL: "https://forge.example", ForgejoAPIToken: "forge-token",
		PendingTTL: 10 * time.Minute, SessionTTL: 15 * time.Minute,
		TelegramAuthMaxAge: 5 * time.Minute, HTTPTimeout: 2 * time.Second,
	}
	telegram := config.TelegramConfig{Token: testBotToken, Username: "NekoForgeBot", AllowedGroups: []int64{-100123}}
	p, err := newImplementation(ctx, language, telegram, cfg, shared)
	if err != nil {
		t.Fatalf("newImplementation() error = %v", err)
	}
	p.now = func() time.Time { return now }
	return p, shared, now
}
