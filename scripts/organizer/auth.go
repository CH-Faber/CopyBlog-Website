package main

import (
	"crypto/subtle"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const sessionCookieName = "faber_organizer_session"

type loginAttempt struct {
	Count int
	Since time.Time
}

type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]loginAttempt
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{attempts: make(map[string]loginAttempt)}
}

func (l *loginLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	value := l.attempts[key]
	if value.Since.IsZero() || now.Sub(value.Since) > 15*time.Minute {
		l.attempts[key] = loginAttempt{Count: 1, Since: now}
		return true
	}
	value.Count++
	l.attempts[key] = value
	return value.Count <= 8
}

func (l *loginLimiter) clear(key string) {
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}

func verifyPassword(hash, password string) bool {
	if hash == "" || password == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func hashPassword(password string) (string, error) {
	value, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(value), err
}

func bearerToken(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(header) < 8 || !strings.EqualFold(header[:7], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(header[7:])
}

func (s *Server) authenticate(r *http.Request) (string, bool) {
	if token := bearerToken(r); token != "" && s.store.validDeviceToken(token) {
		return "bearer", true
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && s.store.validSession(cookie.Value) {
		return "cookie", true
	}
	return "", false
}

func (s *Server) authorized(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kind, ok := s.authenticate(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		if kind == "cookie" && r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			origin := strings.TrimRight(r.Header.Get("Origin"), "/")
			if origin == "" || subtle.ConstantTimeCompare([]byte(origin), []byte(s.cfg.PublicOrigin)) != 1 {
				writeError(w, http.StatusForbidden, "invalid origin")
				return
			}
		}
		next(w, r)
	}
}

func setSessionCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
}
