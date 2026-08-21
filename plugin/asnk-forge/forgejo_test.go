package asnkforge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestHTTPForgejoClientCreatesUser(t *testing.T) {
	client, err := newHTTPForgejoClient("https://forge.example/nested/", "admin-secret", time.Second)
	if err != nil {
		t.Fatalf("newHTTPForgejoClient() error = %v", err)
	}
	client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.Path != "/nested/api/v1/admin/users" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "token admin-secret" {
			t.Errorf("Authorization = %q", got)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		if payload["username"] != "neko" || payload["email"] != "neko@example.test" || payload["password"] != "long-password" {
			t.Errorf("payload = %#v", payload)
		}
		return &http.Response{
			StatusCode: http.StatusCreated,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"id":42,"login":"neko"}`)),
			Request:    r,
		}, nil
	})

	user, err := client.CreateUser(context.Background(), forgejoCreateUser{Username: "neko", Email: "neko@example.test", Password: "long-password"})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if user.ID != 42 || user.Username != "neko" {
		t.Fatalf("user = %#v", user)
	}
}

func TestHTTPForgejoClientReturnsTypedRejection(t *testing.T) {
	client, err := newHTTPForgejoClient("https://forge.example", "admin-secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusUnprocessableEntity,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("private upstream detail")),
			Request:    r,
		}, nil
	})
	_, err = client.CreateUser(context.Background(), forgejoCreateUser{Username: "neko"})
	var apiErr *forgejoAPIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusUnprocessableEntity || !apiErr.retrySafe() {
		t.Fatalf("CreateUser() error = %#v", err)
	}
	if err.Error() == "private upstream detail" {
		t.Fatal("upstream response leaked through error")
	}
}

func TestHTTPForgejoClientLooksUpUser(t *testing.T) {
	client, err := newHTTPForgejoClient("https://forge.example/nested", "admin-secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	client.client.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet || r.URL.Path != "/nested/api/v1/users/neko" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "token admin-secret" {
			t.Fatal("missing administrator token")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"id":42,"login":"neko","email":"neko@example.test"}`)),
			Request:    r,
		}, nil
	})
	user, err := client.LookupUser(context.Background(), "neko")
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != 42 || user.Username != "neko" || user.Email != "neko@example.test" {
		t.Fatalf("user = %#v", user)
	}
}
