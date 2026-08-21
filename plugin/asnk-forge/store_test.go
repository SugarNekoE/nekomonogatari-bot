package asnkforge

import (
	"context"
	"errors"
	"testing"
	"time"

	"forge.asnk.io/sugar/nekomonogatari-bot/data"
)

type legacyForgeIntentModel struct {
	ID                 int64  `gorm:"primaryKey;autoIncrement"`
	CallbackQueryID    string `gorm:"not null;uniqueIndex"`
	TelegramUserID     int64  `gorm:"not null"`
	TelegramChatID     int64  `gorm:"not null"`
	TelegramThreadID   int    `gorm:"not null;default:0"`
	PromptMessageID    int    `gorm:"not null"`
	TelegramUsername   string `gorm:"not null;default:''"`
	DisplayName        string `gorm:"not null"`
	State              string `gorm:"not null"`
	CreatedAtUnix      int64  `gorm:"column:created_at;not null"`
	ExpiresAt          int64  `gorm:"not null"`
	AuthenticatedAt    int64
	CompletedAt        int64
	RequestedUsername  string
	RequestedEmailHash []byte
	ForgejoUserID      int64
	ForgejoUsername    string
}

func (legacyForgeIntentModel) TableName() string { return "asnk-forge_intents" }

func TestForgeStoreRegistrationLifecycle(t *testing.T) {
	ctx := context.Background()
	shared, err := data.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer shared.Close()
	store, err := newForgeStore(ctx, shared)
	if err != nil {
		t.Fatalf("newForgeStore() error = %v", err)
	}
	now := time.Unix(1_900_000_000, 0)
	if err := store.recordPrompt(ctx, -1001, 77, 8, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := store.claim(ctx, "callback-1", 123, -1001, 77, 8, "neko", "@neko", now, 10*time.Minute); err != nil {
		t.Fatalf("claim() error = %v", err)
	}
	if err := store.claim(ctx, "callback-1", 123, -1001, 77, 8, "neko", "@neko", now, 10*time.Minute); !errors.Is(err, errAlreadyClaimed) {
		t.Fatalf("duplicate claim error = %v", err)
	}
	if _, err := store.authenticate(ctx, 999, digestToken("wrong"), digestToken("csrf"), now, 10*time.Minute); !errors.Is(err, errNoPending) {
		t.Fatalf("mismatched authenticate error = %v", err)
	}
	sessionDigest := digestToken("opaque-session")
	csrfDigest := digestToken("csrf-token")
	registration, err := store.authenticate(ctx, 123, sessionDigest, csrfDigest, now, 10*time.Minute)
	if err != nil {
		t.Fatalf("authenticate() error = %v", err)
	}
	if registration.ChatID != -1001 || registration.ThreadID != 8 {
		t.Fatalf("registration = %#v", registration)
	}
	if _, err := store.beginRegistration(ctx, sessionDigest, digestToken("bad-csrf"), "neko", digestEmail("neko@example.test"), now, time.Minute); !errors.Is(err, errInvalidCSRF) {
		t.Fatalf("bad csrf error = %v", err)
	}
	registration, err = store.beginRegistration(ctx, sessionDigest, csrfDigest, "neko", digestEmail("neko@example.test"), now, time.Minute)
	if err != nil {
		t.Fatalf("beginRegistration() error = %v", err)
	}
	if err := store.completeRegistration(ctx, registration, forgejoUser{ID: 44, Username: "neko"}, now); err != nil {
		t.Fatalf("completeRegistration() error = %v", err)
	}
	if _, err := store.session(ctx, sessionDigest, now); !errors.Is(err, errInvalidSession) {
		t.Fatalf("completed session error = %v", err)
	}
	if err := store.claim(ctx, "callback-2", 123, -1001, 77, 8, "neko", "@neko", now, 10*time.Minute); !errors.Is(err, errAlreadyRegistered) {
		t.Fatalf("registered claim error = %v", err)
	}
}

func TestForgeStoreRejectsExpiredPrompt(t *testing.T) {
	ctx := context.Background()
	shared, err := data.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer shared.Close()
	store, err := newForgeStore(ctx, shared)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_900_000_000, 0)
	if err := store.recordPrompt(ctx, -1001, 1, 0, now); err != nil {
		t.Fatal(err)
	}
	if err := store.claim(ctx, "callback", 1, -1001, 1, 0, "", "Neko", now, time.Minute); !errors.Is(err, errPromptExpired) {
		t.Fatalf("claim() error = %v", err)
	}
}

func TestForgeStoreReconcilesDurableCompletion(t *testing.T) {
	ctx := context.Background()
	shared, err := data.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer shared.Close()
	store, err := newForgeStore(ctx, shared)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := store.recordPrompt(ctx, -1001, 5, 0, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := store.claim(ctx, "callback-restart", 505, -1001, 5, 0, "neko", "@neko", now, time.Minute); err != nil {
		t.Fatal(err)
	}
	sessionDigest := digestToken("restart-session")
	csrfDigest := digestToken("restart-csrf")
	registration, err := store.authenticate(ctx, 505, sessionDigest, csrfDigest, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	registration, err = store.beginRegistration(ctx, sessionDigest, csrfDigest, "neko", digestEmail("neko@example.test"), now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := shared.DB().Model(&forgeIntentModel{}).Where("id = ?", registration.ID).Updates(map[string]any{
		"state": "completion_pending", "forgejo_user_id": int64(55), "forgejo_username": "neko",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := newForgeStore(ctx, shared); err != nil {
		t.Fatalf("restart reconciliation: %v", err)
	}
	var account forgeAccountModel
	if err := shared.DB().Where("telegram_user_id = ?", 505).First(&account).Error; err != nil {
		t.Fatal(err)
	}
	if account.ForgejoUserID != 55 || account.ForgejoUsername != "neko" {
		t.Fatalf("account = %#v", account)
	}
}

func TestForgeStorePromotesCrashLeftCreatingIntent(t *testing.T) {
	ctx := context.Background()
	shared, err := data.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer shared.Close()
	if _, err := newForgeStore(ctx, shared); err != nil {
		t.Fatal(err)
	}
	model := forgeIntentModel{
		CallbackQueryID: "crash-callback", TelegramUserID: 606, TelegramChatID: -1001,
		DisplayName: "@crash", State: "creating", CreatedAtUnix: 1, ExpiresAt: time.Now().Add(time.Hour).Unix(),
		RequestedUsername: "crash", RequestedEmailHash: digestEmail("crash@example.test"), OperationStartedAt: time.Now().Unix(),
	}
	if err := shared.DB().Create(&model).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := newForgeStore(ctx, shared); err != nil {
		t.Fatal(err)
	}
	var got forgeIntentModel
	if err := shared.DB().First(&got, model.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.State != "indeterminate" {
		t.Fatalf("state = %q", got.State)
	}
}

func TestForgeStorePromotesLegacyCreatingIntent(t *testing.T) {
	ctx := context.Background()
	shared, err := data.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer shared.Close()
	if err := shared.DB().AutoMigrate(&legacyForgeIntentModel{}); err != nil {
		t.Fatal(err)
	}
	legacy := legacyForgeIntentModel{
		CallbackQueryID: "legacy-callback", TelegramUserID: 607, TelegramChatID: -1001,
		DisplayName: "@legacy", State: "creating", CreatedAtUnix: 1,
		ExpiresAt: time.Now().Add(time.Hour).Unix(), RequestedUsername: "legacy",
		RequestedEmailHash: digestEmail("legacy@example.test"),
	}
	if err := shared.DB().Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := newForgeStore(ctx, shared); err != nil {
		t.Fatal(err)
	}
	var got forgeIntentModel
	if err := shared.DB().First(&got, legacy.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.State != "indeterminate" {
		t.Fatalf("state = %q", got.State)
	}
}

func TestForgeStoreCleansExpiredFlows(t *testing.T) {
	ctx := context.Background()
	shared, err := data.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer shared.Close()
	store, err := newForgeStore(ctx, shared)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	models := []forgeIntentModel{
		{CallbackQueryID: "expired-pending", TelegramUserID: 1, TelegramChatID: -1, DisplayName: "one", State: "pending", CreatedAtUnix: 1, ExpiresAt: now.Add(-time.Minute).Unix()},
		{CallbackQueryID: "expired-auth", TelegramUserID: 2, TelegramChatID: -1, DisplayName: "two", State: "authenticated", CreatedAtUnix: 1, ExpiresAt: now.Add(-time.Minute).Unix()},
	}
	if err := shared.DB().Create(&models).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.recordPrompt(ctx, -1, 9, 0, now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := store.cleanupExpired(ctx, now); err != nil {
		t.Fatal(err)
	}
	var intentCount, promptCount int64
	if err := shared.DB().Model(&forgeIntentModel{}).Count(&intentCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := shared.DB().Model(&forgePromptModel{}).Count(&promptCount).Error; err != nil {
		t.Fatal(err)
	}
	if intentCount != 0 || promptCount != 0 {
		t.Fatalf("remaining intents/prompts = %d/%d", intentCount, promptCount)
	}
}
