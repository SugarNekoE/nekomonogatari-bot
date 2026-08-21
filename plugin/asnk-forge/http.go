package asnkforge

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/go-telegram/bot"
)

//go:embed web/dist
var webFiles embed.FS

var forgejoUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._-]{0,38}[A-Za-z0-9])?$`)

var errForgejoRecoveryMismatch = errors.New("Forgejo user does not match pending registration")

type apiError struct {
	Error string `json:"error"`
}

type sessionResponse struct {
	DisplayName string `json:"displayName"`
	CSRFToken   string `json:"csrfToken"`
}

func (p *implementation) routes() http.Handler {
	dist, err := fs.Sub(webFiles, "web/dist")
	if err != nil {
		panic(fmt.Sprintf("open embedded asnk-forge web bundle: %v", err))
	}
	static := http.StripPrefix("/forge/", http.FileServer(http.FS(dist)))
	mux := http.NewServeMux()
	mux.HandleFunc("/forge", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			methodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		http.Redirect(w, r, "/forge/", http.StatusPermanentRedirect)
	})
	mux.HandleFunc("/forge/api/config", p.handleWebConfig)
	mux.HandleFunc("/forge/api/auth/telegram", p.handleTelegramAuth)
	mux.HandleFunc("/forge/api/session", p.handleSession)
	mux.HandleFunc("/forge/api/register", p.handleRegister)
	mux.HandleFunc("/forge/api/logout", p.handleLogout)
	mux.Handle("/forge/", static)
	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' https://telegram.org 'unsafe-eval'; style-src 'self'; img-src 'self' data: https:; frame-src https://oauth.telegram.org https://telegram.org; connect-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}

func (p *implementation) handleWebConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	noStore(w)
	writeJSON(w, http.StatusOK, struct {
		Language    string      `json:"language"`
		BotUsername string      `json:"botUsername"`
		Messages    webMessages `json:"messages"`
	}{Language: string(p.language), BotUsername: p.telegram.Username, Messages: p.messages.web})
}

func (p *implementation) handleTelegramAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	noStore(w)
	if !p.validOrigin(r) {
		writeJSON(w, http.StatusForbidden, apiError{Error: p.messages.web.TelegramLoginFailed})
		return
	}
	var auth telegramAuthData
	if err := decodeJSON(w, r, &auth, 16<<10); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Error: p.messages.web.TelegramLoginFailed})
		return
	}
	identity, err := verifyTelegramAuth(auth, p.telegram.Token, p.now(), p.cfg.TelegramAuthMaxAge)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, apiError{Error: p.messages.web.TelegramLoginFailed})
		return
	}
	sessionToken, sessionDigest, err := randomToken()
	if err != nil {
		log.Printf("asnk-forge: generate session: %v", err)
		writeJSON(w, http.StatusInternalServerError, apiError{Error: p.messages.web.InternalError})
		return
	}
	csrfToken, csrfDigest, err := randomToken()
	if err != nil {
		log.Printf("asnk-forge: generate csrf token: %v", err)
		writeJSON(w, http.StatusInternalServerError, apiError{Error: p.messages.web.InternalError})
		return
	}
	registration, err := p.store.authenticate(r.Context(), identity.ID, sessionDigest, csrfDigest, p.now(), p.cfg.SessionTTL)
	if err != nil {
		switch {
		case errors.Is(err, errAlreadyRegistered):
			writeJSON(w, http.StatusConflict, apiError{Error: p.messages.web.AlreadyRegistered})
		case errors.Is(err, errNoPending):
			writeJSON(w, http.StatusForbidden, apiError{Error: p.messages.web.NoPending})
		default:
			log.Printf("asnk-forge: authenticate Telegram user: %v", err)
			writeJSON(w, http.StatusInternalServerError, apiError{Error: p.messages.web.InternalError})
		}
		return
	}
	p.setSessionCookie(w, sessionToken)
	writeJSON(w, http.StatusOK, sessionResponse{DisplayName: registration.DisplayName, CSRFToken: csrfToken})
}

func (p *implementation) handleSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	noStore(w)
	digest, ok := p.sessionDigest(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, apiError{Error: p.messages.web.SessionExpired})
		return
	}
	registration, err := p.store.session(r.Context(), digest, p.now())
	if err != nil {
		p.clearSessionCookie(w)
		writeJSON(w, http.StatusUnauthorized, apiError{Error: p.messages.web.SessionExpired})
		return
	}
	csrfToken, csrfDigest, err := randomToken()
	if err != nil {
		log.Printf("asnk-forge: generate csrf token: %v", err)
		writeJSON(w, http.StatusInternalServerError, apiError{Error: p.messages.web.InternalError})
		return
	}
	if err := p.store.rotateCSRF(r.Context(), digest, csrfDigest, p.now()); err != nil {
		log.Printf("asnk-forge: rotate csrf token: %v", err)
		writeJSON(w, http.StatusUnauthorized, apiError{Error: p.messages.web.SessionExpired})
		return
	}
	writeJSON(w, http.StatusOK, sessionResponse{DisplayName: registration.DisplayName, CSRFToken: csrfToken})
}

func (p *implementation) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	noStore(w)
	if !p.validOrigin(r) {
		writeJSON(w, http.StatusForbidden, apiError{Error: p.messages.web.SessionExpired})
		return
	}
	var input forgejoCreateUser
	if err := decodeJSON(w, r, &input, 32<<10); err != nil || !validRegistration(input) {
		writeJSON(w, http.StatusUnprocessableEntity, apiError{Error: p.messages.web.ValidationFailed})
		return
	}
	sessionDigest, ok := p.sessionDigest(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, apiError{Error: p.messages.web.SessionExpired})
		return
	}
	csrf := strings.TrimSpace(r.Header.Get("X-CSRF-Token"))
	if csrf == "" {
		writeJSON(w, http.StatusForbidden, apiError{Error: p.messages.web.SessionExpired})
		return
	}
	csrfDigest := digestToken(csrf)
	registration, err := p.store.beginRegistration(
		r.Context(), sessionDigest, csrfDigest, input.Username, digestEmail(input.Email), p.now(),
		max(p.cfg.SessionTTL, p.cfg.HTTPTimeout+time.Minute),
	)
	if err != nil {
		status := http.StatusUnauthorized
		if errors.Is(err, errInvalidCSRF) {
			status = http.StatusForbidden
		} else if errors.Is(err, errRegistrationBusy) {
			status = http.StatusConflict
		}
		writeJSON(w, status, apiError{Error: p.messages.web.SessionExpired})
		return
	}

	user, createErr := p.forgejo.CreateUser(r.Context(), input)
	storeCtx, cancelStore := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelStore()
	if createErr != nil {
		var apiErr *forgejoAPIError
		if errors.As(createErr, &apiErr) && apiErr.retrySafe() {
			if resetErr := p.store.resetRegistration(storeCtx, registration.ID); resetErr != nil {
				log.Printf("asnk-forge: reset registration after Forgejo rejection: %v", resetErr)
			}
			status := http.StatusBadGateway
			message := p.messages.web.ValidationFailed
			if apiErr.StatusCode == http.StatusConflict || apiErr.StatusCode == http.StatusUnprocessableEntity {
				status = http.StatusConflict
				message = p.messages.web.Conflict
			}
			writeJSON(w, status, apiError{Error: message})
			return
		}
		if markErr := p.store.markRegistrationIndeterminate(storeCtx, registration.ID); markErr != nil {
			log.Printf("asnk-forge: preserve indeterminate registration: %v", markErr)
			p.clearSessionCookie(w)
			writeJSON(w, http.StatusBadGateway, apiError{Error: p.messages.web.UpstreamFailed})
			return
		}
		recoveredUser, recoveryErr := p.lookupIndeterminate(r.Context(), registration)
		if recoveryErr == nil {
			user = recoveredUser
		} else if isForgejoNotFound(recoveryErr) || errors.Is(recoveryErr, errForgejoRecoveryMismatch) {
			if resetErr := p.store.resetRegistration(storeCtx, registration.ID); resetErr != nil {
				log.Printf("asnk-forge: reset unresolved registration: %v", resetErr)
			}
			message := p.messages.web.UpstreamFailed
			status := http.StatusBadGateway
			if errors.Is(recoveryErr, errForgejoRecoveryMismatch) {
				message = p.messages.web.Conflict
				status = http.StatusConflict
			}
			writeJSON(w, status, apiError{Error: message})
			return
		} else {
			log.Printf("asnk-forge: reconcile indeterminate registration: %v", recoveryErr)
			p.clearSessionCookie(w)
			writeJSON(w, http.StatusBadGateway, apiError{Error: p.messages.web.UpstreamFailed})
			return
		}
	}
	if err := p.store.completeRegistration(storeCtx, registration, user, p.now()); err != nil {
		log.Printf("asnk-forge: persist completed registration: %v", err)
		p.clearSessionCookie(w)
		writeJSON(w, http.StatusInternalServerError, apiError{Error: p.messages.web.InternalError})
		return
	}
	p.clearSessionCookie(w)
	p.notifyRegistration(registration, user)
	writeJSON(w, http.StatusCreated, struct {
		Username string `json:"username"`
		Message  string `json:"message"`
	}{Username: user.Username, Message: p.messages.web.Success})
}

func (p *implementation) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}
	noStore(w)
	if !p.validOrigin(r) {
		writeJSON(w, http.StatusForbidden, apiError{Error: p.messages.web.SessionExpired})
		return
	}
	if digest, ok := p.sessionDigest(r); ok {
		if err := p.store.revokeSession(r.Context(), digest); err != nil {
			log.Printf("asnk-forge: revoke session during logout: %v", err)
		}
	}
	p.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (p *implementation) notifyRegistration(registration intent, user forgejoUser) {
	p.botMu.RLock()
	b := p.telegramBot
	p.botMu.RUnlock()
	if b == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), p.cfg.HTTPTimeout)
	defer cancel()
	_, err := b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          registration.ChatID,
		MessageThreadID: registration.ThreadID,
		Text:            fmt.Sprintf(p.messages.registrationNotice, registration.DisplayName, user.Username),
	})
	if err != nil {
		log.Printf("asnk-forge: send registration success: %v", err)
	}
}

func validRegistration(input forgejoCreateUser) bool {
	if !forgejoUsernamePattern.MatchString(input.Username) {
		return false
	}
	if len(input.Email) > 254 || strings.TrimSpace(input.Email) != input.Email {
		return false
	}
	address, err := mail.ParseAddress(input.Email)
	if err != nil || address.Address != input.Email {
		return false
	}
	passwordLength := len([]byte(input.Password))
	return passwordLength >= 8 && passwordLength <= 256
}

func (p *implementation) validOrigin(r *http.Request) bool {
	return r.Header.Get("Origin") == p.origin
}

func (p *implementation) cookieName() string {
	if strings.HasPrefix(p.origin, "https://") {
		return "__Host-asnk-forge-session"
	}
	return "asnk-forge-session"
}

func (p *implementation) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     p.cookieName(),
		Value:    token,
		Path:     "/",
		MaxAge:   max(1, int(p.cfg.SessionTTL/time.Second)),
		Secure:   strings.HasPrefix(p.origin, "https://"),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

func (p *implementation) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     p.cookieName(),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   strings.HasPrefix(p.origin, "https://"),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

func (p *implementation) sessionDigest(r *http.Request) ([]byte, bool) {
	cookie, err := r.Cookie(p.cookieName())
	if err != nil || cookie.Value == "" {
		return nil, false
	}
	return digestToken(cookie.Value), true
}

func randomToken() (string, []byte, error) {
	buffer := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, buffer); err != nil {
		return "", nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(buffer)
	return token, digestToken(token), nil
}

func digestToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func digestEmail(email string) []byte {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return sum[:]
}

func (p *implementation) lookupIndeterminate(ctx context.Context, registration intent) (forgejoUser, error) {
	user, err := p.forgejo.LookupUser(ctx, registration.RequestedUsername)
	if err != nil {
		return forgejoUser{}, err
	}
	if strings.TrimSpace(user.Email) == "" {
		return forgejoUser{}, errors.New("Forgejo lookup did not return an email")
	}
	actualHash := digestEmail(user.Email)
	if !strings.EqualFold(user.Username, registration.RequestedUsername) || len(registration.RequestedEmailHash) != len(actualHash) || subtle.ConstantTimeCompare(registration.RequestedEmailHash, actualHash) != 1 {
		return forgejoUser{}, errForgejoRecoveryMismatch
	}
	return user, nil
}

func isForgejoNotFound(err error) bool {
	var apiErr *forgejoAPIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

func decodeJSON(w http.ResponseWriter, r *http.Request, value any, limit int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request must contain one JSON value")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(true)
	_ = encoder.Encode(value)
}

func noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
}

func methodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
}
