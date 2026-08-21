package asnkforge

import (
	"errors"
	"testing"
	"time"
)

func TestVerifyTelegramAuth(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	valid := signedTelegramAuth(7000000001, now)
	identity, err := verifyTelegramAuth(valid, testBotToken, now, 5*time.Minute)
	if err != nil {
		t.Fatalf("verifyTelegramAuth() error = %v", err)
	}
	if identity.ID != 7000000001 || identity.DisplayName != "@neko_test" {
		t.Fatalf("identity = %#v", identity)
	}

	tests := map[string]telegramAuthData{
		"tampered identity": func() telegramAuthData { value := valid; value.ID = "9"; return value }(),
		"malformed hash":    func() telegramAuthData { value := valid; value.Hash = "no"; return value }(),
		"missing hash":      func() telegramAuthData { value := valid; value.Hash = ""; return value }(),
		"invalid id":        signedTelegramAuth(-1, now),
		"stale":             signedTelegramAuth(1, now.Add(-6*time.Minute)),
		"future":            signedTelegramAuth(1, now.Add(time.Minute)),
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := verifyTelegramAuth(data, testBotToken, now, 5*time.Minute); !errors.Is(err, errInvalidTelegramAuth) {
				t.Fatalf("verifyTelegramAuth() error = %v, want errInvalidTelegramAuth", err)
			}
		})
	}
}
