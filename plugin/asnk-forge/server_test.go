package asnkforge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"forge.asnk.io/sugar/nekomonogatari-bot/config"
)

func TestRequestTrackerWaitsAndRejectsNewRequests(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	tracker := newRequestTracker(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(started)
		<-release
	}))
	served := make(chan struct{})
	go func() {
		tracker.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/forge/", nil))
		close(served)
	}()
	<-started
	tracker.stop()

	waited := make(chan struct{})
	go func() {
		tracker.wait()
		close(waited)
	}()
	select {
	case <-waited:
		t.Fatal("tracker stopped before active request completed")
	case <-time.After(20 * time.Millisecond):
	}

	response := httptest.NewRecorder()
	tracker.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/forge/", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("stopped tracker status = %d", response.Code)
	}
	close(release)
	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("tracker did not finish")
	}
	<-served
}

func TestPeriodicReconciliationHandlesIndeterminateAndCompletionPending(t *testing.T) {
	p, shared, now := testImplementation(t, config.LanguageEnglish)
	p.forgejo = &fakeForgejoClient{lookupUser: forgejoUser{
		ID: 71, Username: "indeterminate", Email: "indeterminate@example.test",
	}}
	models := []forgeIntentModel{
		{
			CallbackQueryID: "indeterminate", TelegramUserID: 701, TelegramChatID: -100123,
			DisplayName: "@indeterminate", State: "indeterminate", CreatedAtUnix: now.Unix(),
			ExpiresAt: now.Add(time.Hour).Unix(), RequestedUsername: "indeterminate",
			RequestedEmailHash: digestEmail("indeterminate@example.test"), OperationStartedAt: now.Unix(),
		},
		{
			CallbackQueryID: "completion", TelegramUserID: 702, TelegramChatID: -100123,
			DisplayName: "@completion", State: "completion_pending", CreatedAtUnix: now.Unix(),
			ExpiresAt: now.Add(time.Hour).Unix(), ForgejoUserID: 72, ForgejoUsername: "completion",
		},
	}
	if err := shared.DB().Create(&models).Error; err != nil {
		t.Fatal(err)
	}
	if err := p.reconcileRegistrations(context.Background()); err != nil {
		t.Fatal(err)
	}
	var accounts []forgeAccountModel
	if err := shared.DB().Order("telegram_user_id").Find(&accounts).Error; err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 2 || accounts[0].ForgejoUserID != 71 || accounts[1].ForgejoUserID != 72 {
		t.Fatalf("accounts = %#v", accounts)
	}
}
