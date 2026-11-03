package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"shippin.cloud/console/internal/config"
)

// Sign-in asks Nomen for the org and Nomen's own APIs in Nomen's spelling.
func TestLoginAsksForNomensScopes(t *testing.T) {
	oa := oauthApp(opsCfg("ops-app"), "console-app", "console.example.test")
	u, _ := url.Parse(oa.AuthCodeURL("st"))
	got := strings.Fields(u.Query().Get("scope"))
	want := map[string]bool{"urn:nomen:iam:user:resourceowner": false, "urn:nomen:iam:org:project:id:nomen:aud": false}
	for _, s := range got {
		if strings.HasPrefix(s, "urn:zitadel:") {
			t.Errorf("asks for %q, the upstream spelling", s)
		}
		if _, ok := want[s]; ok {
			want[s] = true
		}
	}
	for s, seen := range want {
		if !seen {
			t.Errorf("scope %q missing from %v", s, got)
		}
	}
}

// The caller's org comes from Nomen's claim, and org-scoped calls carry Nomen's header.
func TestIdentityReadsNomensOrgAndScopesCallsWithItsHeader(t *testing.T) {
	var orgHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oidc/v1/userinfo":
			_ = json.NewEncoder(w).Encode(map[string]any{"sub": "u1", "name": "Ada",
				"urn:nomen:iam:user:resourceowner:id": "org-1", "urn:nomen:iam:user:resourceowner:name": "Acme"})
		case "/auth/v1/memberships/me/_search":
			_, _ = w.Write([]byte(`{"result":[]}`))
		default:
			orgHeader = r.Header.Get("x-nomen-orgid")
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	f := NewFlow(&config.Config{IDP: config.IDP{Issuer: srv.URL}}, nil)
	f.Client = srv.Client()
	id, err := f.whoami(context.Background(), "tok")
	if err != nil || id.OrgID != "org-1" || id.OrgName != "Acme" {
		t.Fatalf("identity %+v, %v", id, err)
	}
	var out map[string]any
	if err := f.callOrg(context.Background(), http.MethodPost, "/management/v1/users/_search", "tok", "org-2", []byte(`{}`), &out); err != nil {
		t.Fatal(err)
	}
	if orgHeader != "org-2" {
		t.Fatalf("x-nomen-orgid = %q, want org-2", orgHeader)
	}
}

// A site whose issuer runs the upstream build sets idp.dialect, and every reserved name the
// panel sends or reads follows it: scopes, the IdP hint, the org claims and the org header.
func TestDialectSpellsScopesClaimsAndHeader(t *testing.T) {
	var orgHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/oidc/v1/userinfo":
			_ = json.NewEncoder(w).Encode(map[string]any{"sub": "u1",
				"urn:zitadel:iam:user:resourceowner:id": "org-1", "urn:zitadel:iam:user:resourceowner:name": "Acme",
				"urn:zitadel:iam:org:project:roles": map[string]any{"founder": map[string]any{"org-1": "x"}}})
		case "/auth/v1/memberships/me/_search":
			_, _ = w.Write([]byte(`{"result":[]}`))
		default:
			orgHeader = r.Header.Get("x-zitadel-orgid")
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	cfg := &config.Config{IDP: config.IDP{Issuer: srv.URL, ClientID: "app", Dialect: "zitadel",
		Providers: []config.IdPProvider{{ID: "google", Label: "Google", IdPID: "111"}}}, ConsoleHost: "console.example.test"}
	cfg.IdentityDefaults()
	f := NewFlow(cfg, nil)
	f.Client = srv.Client()
	scopes := strings.Join(f.oa.Scopes, " ")
	for _, want := range []string{"urn:zitadel:iam:user:resourceowner", "urn:zitadel:iam:org:project:id:zitadel:aud", "urn:zitadel:iam:org:project:roles"} {
		if !strings.Contains(scopes, want) {
			t.Errorf("scope %q missing from %q", want, scopes)
		}
	}
	if strings.Contains(scopes, "urn:nomen:") {
		t.Errorf("dialect zitadel still asks for %q", scopes)
	}
	if got := idpScope(f.oa.Scopes, cfg.IDP.URN("org:idp:id:111")); !strings.HasSuffix(got, " urn:zitadel:iam:org:idp:id:111") {
		t.Errorf("idp hint %q", got)
	}
	id, err := f.whoami(context.Background(), "tok")
	if err != nil || id.OrgID != "org-1" || id.OrgName != "Acme" || id.Tenant != "org-1" || len(id.ProjectRoles["founder"]) != 1 {
		t.Fatalf("identity %+v, %v", id, err)
	}
	var out map[string]any
	if err := f.callOrg(context.Background(), http.MethodPost, "/management/v1/users/_search", "tok", "org-2", []byte(`{}`), &out); err != nil {
		t.Fatal(err)
	}
	if orgHeader != "org-2" {
		t.Fatalf("x-zitadel-orgid = %q, want org-2", orgHeader)
	}
}
