package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const sessionCookieName = "tb_session"

var errInvalidSessionCookie = errors.New("api: invalid session cookie")

type cookiePayload struct {
	TenantID  int64     `json:"tid"`
	IssuedAt  time.Time `json:"iat"`
	ExpiresAt time.Time `json:"exp"`
}

type sessionSigner struct {
	secret []byte
}

func newSessionSigner(secret string) sessionSigner {
	return sessionSigner{secret: []byte(secret)}
}

func (s sessionSigner) sign(payloadB64 string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(payloadB64))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s sessionSigner) encode(tenantID int64, expiresAt time.Time) (string, error) {
	payload, err := json.Marshal(cookiePayload{TenantID: tenantID, IssuedAt: time.Now().UTC(), ExpiresAt: expiresAt})
	if err != nil {
		return "", fmt.Errorf("api: encode session: %w", err)
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	return payloadB64 + "." + s.sign(payloadB64), nil
}

func (s sessionSigner) decode(value string) (int64, error) {
	payloadB64, sig, ok := strings.Cut(value, ".")
	if !ok {
		return 0, errInvalidSessionCookie
	}
	if !hmac.Equal([]byte(sig), []byte(s.sign(payloadB64))) {
		return 0, errInvalidSessionCookie
	}

	raw, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return 0, errInvalidSessionCookie
	}
	var payload cookiePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return 0, errInvalidSessionCookie
	}
	if payload.ExpiresAt.Before(time.Now()) {
		return 0, errInvalidSessionCookie
	}
	return payload.TenantID, nil
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, signer sessionSigner, tenantID int64, expiresAt time.Time) error {
	value, err := signer.encode(tenantID, expiresAt)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    value,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})
}

func tenantIDFromRequest(r *http.Request, signer sessionSigner) (int64, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return 0, errInvalidSessionCookie
	}
	return signer.decode(cookie.Value)
}
