package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shippin.cloud/console/internal/config"
)

func providerCfg() *config.Config {
	c := opsCfg("ops-app")
	c.IDP.Providers = []config.IdPProvider{{ID: "google", Label: "Google", IdPID: "111"}, {ID: "github", Label: "GitHub", IdPID: "222"}, {ID: "broken", Label: "Broken"}}
	return c
}

func TestProviderLookup(t *testing.T) {
	c := providerCfg()
	if p, ok := c.IDP.Provider("github"); !ok || p.IdPID != "222" {
		t.Fatalf("github: %+v %v", p, ok)
	}
	for _, id := range []string{"", "broken", "GOOGLE", "apple"} {
		if _, ok := c.IDP.Provider(id); ok {
			t.Errorf("%q should not resolve", id)
		}
	}
}

// The hint keeps every base scope and adds exactly one IdP scope.
func TestIdPScope(t *testing.T) {
	_, oa := NewFlow(providerCfg(), nil).app(httptest.NewRequest(http.MethodGet, "/", nil))
	got := strings.Fields(idpScope(oa.Scopes, "111"))
	if len(got) != len(oa.Scopes)+1 || got[len(got)-1] != "urn:nomen:iam:org:idp:id:111" {
		t.Fatalf("scope %v", got)
	}
	for i, s := range oa.Scopes {
		if got[i] != s {
			t.Fatalf("base scope %d changed: %v", i, got)
		}
	}
	if len(oa.Scopes) != len(got)-1 {
		t.Fatal("base scopes mutated")
	}
}

// An unknown provider is refused before any login state is created (nil store would panic).
func TestLoginUnknownProvider(t *testing.T) {
	f := NewFlow(providerCfg(), nil)
	for _, q := range []string{"apple", "broken"} {
		r := httptest.NewRequest(http.MethodGet, "/auth/login?idp="+q, nil)
		r.Host = "console.example.test"
		w := httptest.NewRecorder()
		f.Login(w, r)
		if w.Code != http.StatusBadRequest || w.Header().Get("Location") != "" {
			t.Fatalf("%s: got %d %q", q, w.Code, w.Header().Get("Location"))
		}
	}
}
