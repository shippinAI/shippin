package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/gh"
)

// fakeGitHub answers the App endpoints Shippin uses; the signing-in user can see installation 7.
func fakeGitHub(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/login/oauth/access_token":
			_, _ = w.Write([]byte(`{"access_token":"ghu_x"}`))
		case r.URL.Path == "/user/installations":
			_, _ = w.Write([]byte(`{"installations":[{"id":7,"account":{"login":"operator","type":"User"},"repository_selection":"selected"}]}`))
		case strings.HasSuffix(r.URL.Path, "/access_tokens"):
			fmt.Fprintf(w, `{"token":"ghs_test","expires_at":"%s"}`, time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
		case r.URL.Path == "/installation/repositories":
			_, _ = w.Write([]byte(`{"repositories":[{"id":42,"full_name":"operator/app","private":true,"default_branch":"main"}]}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func githubAPI(t *testing.T) (*API, *db.DB, db.Tenant, *caller) {
	a, d, tn := notifyAPI(t)
	// The fake's installation 7 is shared across runs of this disposable database.
	if _, err := d.Pool.Exec(context.Background(), `DELETE FROM project_sources WHERE installation_id=7;
	  DELETE FROM github_installations WHERE id=7`); err != nil {
		t.Fatal(err)
	}
	srv := fakeGitHub(t)
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	p := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	a.cfg.GitHub = config.GitHub{Enabled: true, AppSlug: "shippin", APIBase: srv.URL, WebBase: srv.URL, StateMinutes: 15,
		MaxRepos: 100, RefreshMinutes: 45, PollMinutes: 10, WebhookSecret: "whs", TokenPermissions: map[string]string{"contents": "write"}}
	a.cfg.SessionKey = "test-session-key"
	app, err := gh.New("Iv1.x", "cs", string(p), srv.URL, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	a.gh = app
	owner := &caller{id: &auth.Identity{Sub: "sub-" + uuid.NewString()}, managers: []string{"org_owner"},
		tenants: []tenantView{{Tenant: tn, Roles: []string{"org_owner"}}}}
	return a, d, tn, owner
}

func setup(a *API, c *caller, q url.Values) *httptest.ResponseRecorder {
	a.resolveCaller = func(http.ResponseWriter, *http.Request) (*caller, bool) { return c, true }
	w := httptest.NewRecorder()
	a.githubSetup(w, httptest.NewRequest("GET", "/v1/github/setup?"+q.Encode(), nil))
	return w
}

func TestGitHubSetupVerifiesStateAndInstallation(t *testing.T) {
	a, d, tn, owner := githubAPI(t)
	ctx := context.Background()
	acct, _ := d.UpsertAccount(ctx, owner.id.Sub, "")
	state := a.stateFor(tn.ID, acct, time.Now())
	if w := setup(a, owner, url.Values{"state": {state + "x"}, "installation_id": {"7"}, "code": {"c"}}); w.Code != 400 {
		t.Fatalf("tampered state: %d", w.Code)
	}
	if w := setup(a, owner, url.Values{"state": {a.stateFor(tn.ID, acct, time.Now().Add(-time.Hour))}, "installation_id": {"7"}, "code": {"c"}}); w.Code != 400 {
		t.Fatalf("expired state: %d", w.Code)
	}
	if w := setup(a, owner, url.Values{"state": {state}, "installation_id": {"8"}, "code": {"c"}}); w.Code != 403 {
		t.Fatalf("forged installation id: %d", w.Code)
	}
	other := &caller{id: &auth.Identity{Sub: "sub-" + uuid.NewString()}, managers: owner.managers, tenants: owner.tenants}
	if w := setup(a, other, url.Values{"state": {state}, "installation_id": {"7"}, "code": {"c"}}); w.Code != 400 {
		t.Fatalf("state issued to someone else: %d", w.Code)
	}
	if w := setup(a, owner, url.Values{"state": {state}, "installation_id": {"7"}, "code": {"c"}}); w.Code != 302 {
		t.Fatalf("setup: %d %s", w.Code, w.Body)
	}
	if ins, _ := d.Installations(ctx, tn.ID); len(ins) != 1 || ins[0].AccountLogin != "operator" {
		t.Fatalf("installation %+v", ins)
	}
}

func TestGitHubWebhook(t *testing.T) {
	a, d, tn, _ := githubAPI(t)
	ctx := context.Background()
	inst := time.Now().UnixNano() % 1_000_000_000
	_ = d.BindInstallation(ctx, db.GitHubInstallation{ID: inst, TenantID: tn.ID}, "")
	body := fmt.Sprintf(`{"action":"deleted","installation":{"id":%d}}`, inst)
	send := func(sig, id string) int {
		r := httptest.NewRequest("POST", "/v1/github/webhook", strings.NewReader(body))
		r.Header.Set("X-Hub-Signature-256", sig)
		r.Header.Set("X-GitHub-Delivery", id)
		r.Header.Set("X-GitHub-Event", "installation")
		w := httptest.NewRecorder()
		a.githubWebhook(w, r)
		return w.Code
	}
	m := hmac.New(sha256.New, []byte("whs"))
	m.Write([]byte(body))
	good := "sha256=" + hex.EncodeToString(m.Sum(nil))
	if code := send("sha256=00", uuid.NewString()); code != 401 {
		t.Fatalf("bad signature: %d", code)
	}
	id := uuid.NewString()
	if send(good, id) != 204 || send(good, id) != 204 {
		t.Fatal("delivery and replay are acknowledged")
	}
	if ins, _ := d.Installations(ctx, tn.ID); len(ins) != 0 {
		t.Fatal("installation.deleted applied")
	}
}
