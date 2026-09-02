package asnkforge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type forgejoCreateUser struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type forgejoUser struct {
	ID       int64  `json:"id"`
	Username string `json:"login"`
	Email    string `json:"email"`
}

type forgejoClient interface {
	CreateUser(context.Context, forgejoCreateUser) (forgejoUser, error)
	LookupUser(context.Context, string) (forgejoUser, error)
}

type forgejoAPIError struct {
	StatusCode int
}

func (e *forgejoAPIError) Error() string {
	return fmt.Sprintf("forgejo API returned HTTP %d", e.StatusCode)
}

func (e *forgejoAPIError) retrySafe() bool {
	return e.StatusCode >= http.StatusBadRequest && e.StatusCode < http.StatusInternalServerError
}

type httpForgejoClient struct {
	createEndpoint string
	usersEndpoint  string
	token          string
	client         *http.Client
}

func newHTTPForgejoClient(rawURL, token string, timeout time.Duration) (*httpForgejoClient, error) {
	base, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, errors.New("forgejo URL must be absolute")
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return nil, errors.New("forgejo URL must use HTTP or HTTPS")
	}
	if base.User != nil {
		return nil, errors.New("forgejo URL must not contain user information")
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("forgejo API token is required")
	}
	if timeout <= 0 {
		return nil, errors.New("forgejo HTTP timeout must be positive")
	}
	base.RawQuery = ""
	base.Fragment = ""
	apiPath := strings.TrimRight(base.Path, "/") + "/api/v1"
	createURL := *base
	createURL.Path = apiPath + "/admin/users"
	usersURL := *base
	usersURL.Path = apiPath + "/users/"
	return &httpForgejoClient{
		createEndpoint: createURL.String(),
		usersEndpoint:  usersURL.String(),
		token:          strings.TrimSpace(token),
		client:         &http.Client{Timeout: timeout},
	}, nil
}

func (c *httpForgejoClient) CreateUser(ctx context.Context, input forgejoCreateUser) (forgejoUser, error) {
	payload := struct {
		Username           string `json:"username"`
		Email              string `json:"email"`
		Password           string `json:"password"`
		MustChangePassword bool   `json:"must_change_password"`
		SendNotify         bool   `json:"send_notify"`
	}{
		Username: input.Username,
		Email:    input.Email,
		Password: input.Password,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return forgejoUser{}, fmt.Errorf("encode Forgejo user: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.createEndpoint, bytes.NewReader(body))
	if err != nil {
		return forgejoUser{}, fmt.Errorf("create Forgejo request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "token "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return forgejoUser{}, fmt.Errorf("call Forgejo API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return forgejoUser{}, &forgejoAPIError{StatusCode: resp.StatusCode}
	}

	var result forgejoUser
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 64<<10))
	if err := decoder.Decode(&result); err != nil {
		return forgejoUser{}, fmt.Errorf("decode Forgejo user: %w", err)
	}
	if result.ID <= 0 {
		return forgejoUser{}, errors.New("forgejo returned a user without an ID")
	}
	if result.Username == "" {
		result.Username = input.Username
	}
	return result, nil
}

func (c *httpForgejoClient) LookupUser(ctx context.Context, username string) (forgejoUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.usersEndpoint+url.PathEscape(username), nil)
	if err != nil {
		return forgejoUser{}, fmt.Errorf("create Forgejo lookup request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "token "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return forgejoUser{}, fmt.Errorf("call Forgejo lookup API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return forgejoUser{}, &forgejoAPIError{StatusCode: resp.StatusCode}
	}
	var result forgejoUser
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&result); err != nil {
		return forgejoUser{}, fmt.Errorf("decode Forgejo lookup user: %w", err)
	}
	if result.ID <= 0 || result.Username == "" {
		return forgejoUser{}, errors.New("forgejo lookup returned an invalid user")
	}
	return result, nil
}
