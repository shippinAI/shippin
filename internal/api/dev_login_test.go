package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestDevSignInIsRefusedOffADevelopmentAddressOrForAnUnlistedAccount(t *testing.T) {
	t.Setenv("SHIPPIN_DEV_ACCOUNTS", "boss@example.test=admin")
	t.Setenv("SHIPPIN_DEV_ACCOUNTS_PASSWORD", "a-long-dev-password")
	a := &API{}
	for _, c := range []struct{ host, email string }{
		{"panel.example.com", "boss@example.test"},
		{"localhost:18443", "someone@example.test"},
		{"localhost:18443", ""},
	} {
		req := httptest.NewRequest(http.MethodPost, "/auth/dev", strings.NewReader("email="+url.QueryEscape(c.email)))
		req.Host = c.host
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		a.devLogin(rec, req)
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/login?error=dev_unavailable" || rec.Header().Get("Set-Cookie") != "" {
			t.Errorf("%s as %q: %d to %q, cookie %q", c.host, c.email, rec.Code, rec.Header().Get("Location"), rec.Header().Get("Set-Cookie"))
		}
	}
}

func TestDevSignInButtonsAreOnlyOfferedWhereTheyWork(t *testing.T) {
	t.Setenv("SHIPPIN_DEV_ACCOUNTS", "boss@example.test=admin,user@example.test=customer")
	t.Setenv("SHIPPIN_DEV_ACCOUNTS_PASSWORD", "a-long-dev-password")
	if got := devSignIn("localhost:18443", true); len(got) != 2 || got[0]["email"] != "boss@example.test" || got[0]["label"] != "Admin" {
		t.Fatalf("%v", got)
	}
	if got := devSignIn("panel.example.com", true); len(got) != 0 {
		t.Fatalf("offered on a public address: %v", got)
	}
	if got := devSignIn("localhost", false); len(got) != 0 {
		t.Fatalf("offered where local sign-in is off: %v", got)
	}
}
