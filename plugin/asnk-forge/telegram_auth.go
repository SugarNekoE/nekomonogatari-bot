package asnkforge

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

var errInvalidTelegramAuth = errors.New("invalid telegram authorization")

type telegramAuthData struct {
	ID        string `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
	PhotoURL  string `json:"photo_url,omitempty"`
	AuthDate  string `json:"auth_date"`
	Hash      string `json:"hash"`
}

type telegramIdentity struct {
	ID          int64
	Username    string
	DisplayName string
}

func verifyTelegramAuth(data telegramAuthData, botToken string, now time.Time, maxAge time.Duration) (telegramIdentity, error) {
	if strings.TrimSpace(botToken) == "" || maxAge <= 0 || data.ID == "" || data.AuthDate == "" || data.Hash == "" {
		return telegramIdentity{}, errInvalidTelegramAuth
	}

	fields := map[string]string{
		"id":        data.ID,
		"auth_date": data.AuthDate,
	}
	for key, value := range map[string]string{
		"first_name": data.FirstName,
		"last_name":  data.LastName,
		"username":   data.Username,
		"photo_url":  data.PhotoURL,
	} {
		if value != "" {
			fields[key] = value
		}
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key+"="+fields[key])
	}

	provided, err := hex.DecodeString(data.Hash)
	if err != nil || len(provided) != sha256.Size {
		return telegramIdentity{}, errInvalidTelegramAuth
	}
	secret := sha256.Sum256([]byte(botToken))
	mac := hmac.New(sha256.New, secret[:])
	_, _ = mac.Write([]byte(strings.Join(lines, "\n")))
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return telegramIdentity{}, errInvalidTelegramAuth
	}

	userID, err := strconv.ParseInt(data.ID, 10, 64)
	if err != nil || userID <= 0 {
		return telegramIdentity{}, errInvalidTelegramAuth
	}
	authUnix, err := strconv.ParseInt(data.AuthDate, 10, 64)
	if err != nil || authUnix <= 0 {
		return telegramIdentity{}, errInvalidTelegramAuth
	}
	authTime := time.Unix(authUnix, 0)
	if authTime.After(now.Add(30*time.Second)) || now.Sub(authTime) > maxAge {
		return telegramIdentity{}, errInvalidTelegramAuth
	}

	displayName := strings.TrimSpace(data.FirstName + " " + data.LastName)
	if data.Username != "" {
		displayName = "@" + data.Username
	}
	if displayName == "" {
		displayName = fmt.Sprintf("Telegram user %d", userID)
	}
	return telegramIdentity{ID: userID, Username: data.Username, DisplayName: displayName}, nil
}
