package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shippin.cloud/console/internal/config"
)

func opsCfg(clientID string) *config.Config {
	return &config.Config{ConsoleHost: "console.example.test", IDP: config.IDP{Issuer: "https://id.example.test", ClientID: "console-app"},
		Operator: config.Operator{Host: "ops.example.test", ClientID: clientID}}
}

// The operator origin is its own app: its own client id and callback, picked by Host only.
func TestOperatorAudienceByHost(t *testing.T) {
	f := NewFlow(opsCfg("ops-app"), nil)
	for host, want := range map[string]string{"ops.example.test": AudOperator, "OPS.example.test:443": AudOperator,
		"console.example.test": "", "ops.example.test.evil.test": "", "": ""} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Host = host
		if got := f.Audience(r); got != want {
			t.Errorf("Audience(%q) = %q, want %q", host, got, want)
		}
		aud, oa := f.app(r)
		wantID, wantCB := "console-app", "https://console.example.test/auth/callback"
		if aud == AudOperator {
			wantID, wantCB = "ops-app", "https://ops.example.test/auth/callback"
		}
		if oa.ClientID != wantID || oa.RedirectURL != wantCB {
			t.Errorf("%q: app %s %s", host, oa.ClientID, oa.RedirectURL)
		}
	}
	if NewFlow(&config.Config{}, nil).Audience(httptest.NewRequest(http.MethodGet, "/", nil)) != "" {
		t.Error("no operator host configured: everything is the console")
	}
}

// Until the operator app is registered at the IdP, its sign-in refuses instead of falling back
// to the console app.
func TestOperatorLoginNeedsItsOwnApp(t *testing.T) {
	f := NewFlow(opsCfg(""), nil)
	r := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	r.Host = "ops.example.test"
	w := httptest.NewRecorder()
	f.Login(w, r)
	if w.Code != http.StatusServiceUnavailable || strings.Contains(w.Header().Get("Location"), "console-app") {
		t.Fatalf("got %d %q", w.Code, w.Header().Get("Location"))
	}
}
