package main

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const sessionCookieName = "ht_session"
const sessionMaxAge = 24 * time.Hour
const sessionIdleTimeout = 30 * time.Minute

// Rate limiting
const (
	loginMaxAttempts = 5
	loginLockoutTime = 5 * time.Minute
)

// TLS flag — set by main() before server starts
var secureCookies bool

type session struct {
	username     string
	password     string
	authMethod   string
	subject      string
	sudo         bool
	createdAt    time.Time
	lastActivity time.Time
}

type loginAttempt struct {
	failures int
	lastFail time.Time
}

var (
	sessions   = make(map[string]session)
	sessionsMu sync.Mutex

	loginAttempts   = make(map[string]*loginAttempt)
	loginAttemptsMu sync.Mutex
)

// getClientIP extracts the client IP from the request.
func getClientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		// Take the first IP in the chain
		if idx := net.ParseIP(fwd); idx != nil {
			return fwd
		}
		// May have multiple comma-separated values
		for _, part := range splitComma(fwd) {
			part = trimSpace(part)
			if net.ParseIP(part) != nil {
				return part
			}
		}
	}
	if real := r.Header.Get("X-Real-IP"); real != "" {
		if net.ParseIP(real) != nil {
			return real
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func splitComma(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func trimSpace(s string) string {
	for len(s) > 0 && s[0] == ' ' {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == ' ' {
		s = s[:len(s)-1]
	}
	return s
}

// isRateLimited checks if an IP has exceeded the login attempt limit.
func isRateLimited(ip string) bool {
	loginAttemptsMu.Lock()
	defer loginAttemptsMu.Unlock()

	attempt, ok := loginAttempts[ip]
	if !ok {
		return false
	}
	// Reset if lockout period has passed
	if time.Since(attempt.lastFail) > loginLockoutTime {
		delete(loginAttempts, ip)
		return false
	}
	return attempt.failures >= loginMaxAttempts
}

// recordFailedLogin records a failed login attempt for an IP.
func recordFailedLogin(ip string) {
	loginAttemptsMu.Lock()
	defer loginAttemptsMu.Unlock()

	attempt, ok := loginAttempts[ip]
	if !ok {
		loginAttempts[ip] = &loginAttempt{failures: 1, lastFail: time.Now()}
		return
	}
	// Reset count if lockout period has passed
	if time.Since(attempt.lastFail) > loginLockoutTime {
		attempt.failures = 1
	} else {
		attempt.failures++
	}
	attempt.lastFail = time.Now()
}

// clearFailedLogins resets the failed login counter for an IP.
func clearFailedLogins(ip string) {
	loginAttemptsMu.Lock()
	delete(loginAttempts, ip)
	loginAttemptsMu.Unlock()
}

// startSessionCleanup runs a background goroutine that purges expired sessions.
func startSessionCleanup() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now()
			sessionsMu.Lock()
			for token, sess := range sessions {
				if now.Sub(sess.createdAt) > sessionMaxAge || now.Sub(sess.lastActivity) > sessionIdleTimeout {
					delete(sessions, token)
				}
			}
			sessionsMu.Unlock()

			// Also clean up old rate limit entries
			loginAttemptsMu.Lock()
			for ip, attempt := range loginAttempts {
				if now.Sub(attempt.lastFail) > loginLockoutTime {
					delete(loginAttempts, ip)
				}
			}
			loginAttemptsMu.Unlock()
		}
	}()
}

func createSession(username, password string) string {
	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)

	now := time.Now()
	sessionsMu.Lock()
	sessions[token] = session{
		username:     username,
		password:     password,
		authMethod:   "pam",
		createdAt:    now,
		lastActivity: now,
	}
	sessionsMu.Unlock()

	return token
}

// validateSession checks token validity, expiry, and idle timeout.
// It also updates lastActivity on success.
func validateSession(token string) (session, bool) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()

	sess, ok := sessions[token]
	if !ok {
		return session{}, false
	}
	now := time.Now()
	if now.Sub(sess.createdAt) > sessionMaxAge || now.Sub(sess.lastActivity) > sessionIdleTimeout {
		delete(sessions, token)
		return session{}, false
	}
	// Touch activity
	sess.lastActivity = now
	sessions[token] = sess
	return sess, true
}

func getSession(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return "", false
	}
	sess, ok := validateSession(cookie.Value)
	if !ok {
		return "", false
	}
	return sess.username, true
}

func getSessionPassword(r *http.Request) (string, string, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return "", "", false
	}
	sess, ok := validateSession(cookie.Value)
	if !ok {
		return "", "", false
	}
	if sess.password == "" || sess.authMethod != "pam" {
		return "", "", false
	}
	return sess.username, sess.password, true
}

func isSudoEnabled(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	sessionsMu.Lock()
	sess, ok := sessions[cookie.Value]
	sessionsMu.Unlock()
	if !ok {
		return false
	}
	return sess.sudo
}

func toggleSudo(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	sess, ok := sessions[cookie.Value]
	if !ok {
		return false
	}
	sess.sudo = !sess.sudo
	sessions[cookie.Value] = sess
	return sess.sudo
}

func destroySession(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil {
		sessionsMu.Lock()
		delete(sessions, cookie.Value)
		sessionsMu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secureCookies,
	})
}

// checkCSRFOrigin validates that POST requests originate from the same host.
// It checks the Origin header (set by browsers on cross-origin requests) and
// falls back to Referer. Requests with no Origin/Referer are allowed so that
// curl and direct API usage continue to work; SameSite=Strict cookies already
// block the primary CSRF attack vector in modern browsers.
func checkCSRFOrigin(r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
		return true
	}
	check := func(raw string) bool {
		u, err := url.Parse(raw)
		if err != nil {
			return false
		}
		return u.Host == r.Host
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		return check(origin)
	}
	if referer := r.Header.Get("Referer"); referer != "" {
		return check(referer)
	}
	// No header present — allow (curl/API usage)
	return true
}

func requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := getSession(r); !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if !checkCSRFOrigin(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionMaxAge.Seconds()),
		HttpOnly: true,
		Secure:   secureCookies,
		SameSite: http.SameSiteStrictMode,
	})
}
