package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateAndGetSession(t *testing.T) {
	token := createSession("testuser", "testpass")
	if token == "" {
		t.Fatal("createSession returned empty token")
	}
	if len(token) < 32 {
		t.Errorf("token too short: %d chars", len(token))
	}

	// Create a request with the session cookie
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "ht_session", Value: token})

	username, ok := getSession(req)
	if !ok {
		t.Fatal("getSession returned false for valid session")
	}
	if username != "testuser" {
		t.Errorf("getSession username = %q, want %q", username, "testuser")
	}
}

func TestGetSessionInvalid(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "ht_session", Value: "invalid_token"})

	_, ok := getSession(req)
	if ok {
		t.Error("getSession should return false for invalid token")
	}
}

func TestGetSessionNoCookie(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)

	_, ok := getSession(req)
	if ok {
		t.Error("getSession should return false when no cookie")
	}
}

func TestGetSessionPassword(t *testing.T) {
	token := createSession("admin", "secret123")

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "ht_session", Value: token})

	username, password, ok := getSessionPassword(req)
	if !ok {
		t.Fatal("getSessionPassword returned false for valid session")
	}
	if username != "admin" {
		t.Errorf("username = %q, want %q", username, "admin")
	}
	if password != "secret123" {
		t.Errorf("password = %q, want %q", password, "secret123")
	}
}

func TestSudoToggle(t *testing.T) {
	token := createSession("user1", "pass1")

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "ht_session", Value: token})

	// Should start disabled
	if isSudoEnabled(req) {
		t.Error("sudo should be disabled initially")
	}

	// Toggle on
	toggleSudo(req)
	if !isSudoEnabled(req) {
		t.Error("sudo should be enabled after toggle")
	}

	// Toggle off
	toggleSudo(req)
	if isSudoEnabled(req) {
		t.Error("sudo should be disabled after second toggle")
	}
}

func TestRequireAuthRedirect(t *testing.T) {
	handler := requireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Request without session should redirect to /login
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("expected redirect 303, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %q", loc)
	}
}

func TestRequireAuthValid(t *testing.T) {
	handler := requireAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	token := createSession("validuser", "validpass")
	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: "ht_session", Value: token})
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}
