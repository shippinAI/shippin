package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"shippin.cloud/console/internal/config"
)

func TestReauthWithNoIdPGoesToTheLocalSignIn(t *testing.T) {
	f := &Flow{cfg: &config.Config{}}
	rec := httptest.NewRecorder()
	f.Login(rec, httptest.NewRequest(http.MethodGet, "/auth/login?reauth=1&rd=/admin", nil))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/login" {
		t.Fatalf("reauth: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	rec = httptest.NewRecorder()
	f.Login(rec, httptest.NewRequest(http.MethodGet, "/auth/login", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("an ordinary login with no IdP still says it is not set up: %d", rec.Code)
	}
}
