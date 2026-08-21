package asnkforge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"forge.asnk.io/sugar/nekomonogatari-bot/config"
)

type fakeForgejoClient struct {
	input      forgejoCreateUser
	user       forgejoUser
	err        error
	lookupUser forgejoUser
	lookupErr  error
}

func (f *fakeForgejoClient) LookupUser(_ context.Context, _ string) (forgejoUser, error) {
	return f.lookupUser, f.lookupErr
}

func (f *fakeForgejoClient) CreateUser(_ context.Context, input forgejoCreateUser) (forgejoUser, error) {
	f.input = input
	return f.user, f.err
}

func TestHTTPRegistrationFlow(t *testing.T) {
	p, shared, now := testImplementation(t, config.LanguageEnglish)
	fake := &fakeForgejoClient{user: forgejoUser{ID: 88, Username: "neko"}}
	p.forgejo = fake
	ctx := context.Background()
	if err := p.store.recordPrompt(ctx, -100123, 77, 4, now.Add(p.cfg.PendingTTL)); err != nil {
		t.Fatal(err)
	}
	if err := p.store.claim(ctx, "callback", 777, -100123, 77, 4, "neko_test", "@neko_test", now, p.cfg.PendingTTL); err != nil {
		t.Fatal(err)
	}

	authBody, _ := json.Marshal(signedTelegramAuth(777, now))
	authRequest := httptest.NewRequest(http.MethodPost, "https://register.example/forge/api/auth/telegram", bytes.NewReader(authBody))
	authRequest.Header.Set("Origin", "https://register.example")
	authRequest.Header.Set("Content-Type", "application/json")
	authResponse := httptest.NewRecorder()
	p.handler.ServeHTTP(authResponse, authRequest)
	if authResponse.Code != http.StatusOK {
		t.Fatalf("auth status = %d, body = %s", authResponse.Code, authResponse.Body.String())
	}
	var session sessionResponse
	if err := json.NewDecoder(authResponse.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	if session.DisplayName != "@neko_test" || session.CSRFToken == "" {
		t.Fatalf("session = %#v", session)
	}
	cookies := authResponse.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookies = %#v", cookies)
	}
	if strings.Contains(p.webURL, "777") || strings.Contains(p.webURL, "callback") || p.webURL != "https://register.example/forge/" {
		t.Fatalf("web URL contains identity state: %q", p.webURL)
	}

	registrationBody := []byte(`{"username":"neko","email":"neko@example.test","password":"safe-password"}`)
	badCSRFRequest := httptest.NewRequest(http.MethodPost, "https://register.example/forge/api/register", bytes.NewReader(registrationBody))
	badCSRFRequest.Header.Set("Origin", "https://register.example")
	badCSRFRequest.Header.Set("Content-Type", "application/json")
	badCSRFRequest.Header.Set("X-CSRF-Token", "wrong")
	badCSRFRequest.AddCookie(cookies[0])
	badCSRFResponse := httptest.NewRecorder()
	p.handler.ServeHTTP(badCSRFResponse, badCSRFRequest)
	if badCSRFResponse.Code != http.StatusForbidden {
		t.Fatalf("bad csrf status = %d", badCSRFResponse.Code)
	}

	registerRequest := httptest.NewRequest(http.MethodPost, "https://register.example/forge/api/register", bytes.NewReader(registrationBody))
	registerRequest.Header.Set("Origin", "https://register.example")
	registerRequest.Header.Set("Content-Type", "application/json")
	registerRequest.Header.Set("X-CSRF-Token", session.CSRFToken)
	registerRequest.AddCookie(cookies[0])
	registerResponse := httptest.NewRecorder()
	p.handler.ServeHTTP(registerResponse, registerRequest)
	if registerResponse.Code != http.StatusCreated {
		t.Fatalf("register status = %d, body = %s", registerResponse.Code, registerResponse.Body.String())
	}
	if fake.input.Username != "neko" || fake.input.Email != "neko@example.test" || fake.input.Password != "safe-password" {
		t.Fatalf("Forgejo input = %#v", fake.input)
	}
	var account forgeAccountModel
	if err := shared.DB().Where("telegram_user_id = ?", 777).First(&account).Error; err != nil {
		t.Fatalf("registered account lookup: %v", err)
	}
	if account.ForgejoUserID != 88 || account.ForgejoUsername != "neko" {
		t.Fatalf("account = %#v", account)
	}
}

func TestHTTPRejectsDifferentTelegramAccount(t *testing.T) {
	p, _, now := testImplementation(t, config.LanguageChinese)
	ctx := context.Background()
	if err := p.store.recordPrompt(ctx, -100123, 1, 0, now.Add(p.cfg.PendingTTL)); err != nil {
		t.Fatal(err)
	}
	if err := p.store.claim(ctx, "callback", 111, -100123, 1, 0, "first", "@first", now, p.cfg.PendingTTL); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(signedTelegramAuth(222, now))
	request := httptest.NewRequest(http.MethodPost, "https://register.example/forge/api/auth/telegram", bytes.NewReader(body))
	request.Header.Set("Origin", "https://register.example")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	p.handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "没有有效") {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestHTTPRecoversIndeterminateForgejoCreate(t *testing.T) {
	p, shared, now := testImplementation(t, config.LanguageEnglish)
	p.forgejo = &fakeForgejoClient{
		err:        errors.New("response was lost"),
		lookupUser: forgejoUser{ID: 99, Username: "recovered", Email: "recovered@example.test"},
	}
	ctx := context.Background()
	if err := p.store.recordPrompt(ctx, -100123, 77, 0, now.Add(p.cfg.PendingTTL)); err != nil {
		t.Fatal(err)
	}
	if err := p.store.claim(ctx, "callback-recovery", 707, -100123, 77, 0, "recovered", "@recovered", now, p.cfg.PendingTTL); err != nil {
		t.Fatal(err)
	}
	authBody, _ := json.Marshal(signedTelegramAuth(707, now))
	authRequest := httptest.NewRequest(http.MethodPost, "https://register.example/forge/api/auth/telegram", bytes.NewReader(authBody))
	authRequest.Header.Set("Origin", "https://register.example")
	authResponse := httptest.NewRecorder()
	p.handler.ServeHTTP(authResponse, authRequest)
	if authResponse.Code != http.StatusOK {
		t.Fatalf("auth status = %d, body = %s", authResponse.Code, authResponse.Body.String())
	}
	var session sessionResponse
	if err := json.NewDecoder(authResponse.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}

	body := []byte(`{"username":"recovered","email":"recovered@example.test","password":"safe-password"}`)
	request := httptest.NewRequest(http.MethodPost, "https://register.example/forge/api/register", bytes.NewReader(body))
	request.Header.Set("Origin", "https://register.example")
	request.Header.Set("X-CSRF-Token", session.CSRFToken)
	request.AddCookie(authResponse.Result().Cookies()[0])
	response := httptest.NewRecorder()
	p.handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("register status = %d, body = %s", response.Code, response.Body.String())
	}
	var account forgeAccountModel
	if err := shared.DB().Where("telegram_user_id = ?", 707).First(&account).Error; err != nil {
		t.Fatal(err)
	}
	if account.ForgejoUserID != 99 || account.ForgejoUsername != "recovered" {
		t.Fatalf("account = %#v", account)
	}
}

func TestEmbeddedWebAndSecurityHeaders(t *testing.T) {
	p, _, _ := testImplementation(t, config.LanguageEnglish)
	request := httptest.NewRequest(http.MethodGet, "https://register.example/forge/", nil)
	response := httptest.NewRecorder()
	p.handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	body, _ := io.ReadAll(response.Body)
	if !bytes.Contains(body, []byte("/forge/assets/")) {
		t.Fatalf("embedded index is not the built Vite bundle: %s", body)
	}
	if response.Header().Get("Content-Security-Policy") == "" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("security headers = %#v", response.Header())
	}
}
