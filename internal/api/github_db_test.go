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
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/platform/config"
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
	// The App is built from the section, here still the file's: its keys as the environment gives them.
	a.cfg.GitHub = config.GitHub{Enabled: true, AppSlug: "shippin", APIBase: srv.URL, WebBase: srv.URL, StateMinutes: 15,
		MaxRepos: 100, RefreshMinutes: 45, PollMinutes: 10, WebhookSecret: "whs", TokenPermissions: map[string]string{"contents": "write"},
		ClientID: "Iv1.x", ClientSecret: "cs", PrivateKey: string(p)}
	a.cfg.SessionKey = "test-session-key"
	if app, _ := a.github(context.Background()); app == nil {
		t.Fatal("the App is built from the file's section")
	}
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
	state := a.stateFor(ctx, tn.ID, acct, "runners", time.Now())
	if w := setup(a, owner, url.Values{"state": {state + "x"}, "installation_id": {"7"}, "code": {"c"}}); w.Code != 400 {
		t.Fatalf("tampered state: %d", w.Code)
	}
	if w := setup(a, owner, url.Values{"state": {a.stateFor(ctx, tn.ID, acct, "", time.Now().Add(-time.Hour))}, "installation_id": {"7"}, "code": {"c"}}); w.Code != 400 {
		t.Fatalf("expired state: %d", w.Code)
	}
	if w := setup(a, owner, url.Values{"state": {state}, "installation_id": {"8"}, "code": {"c"}}); w.Code != 403 {
		t.Fatalf("forged installation id: %d", w.Code)
	}
	other := &caller{id: &auth.Identity{Sub: "sub-" + uuid.NewString()}, managers: owner.managers, tenants: owner.tenants}
	if w := setup(a, other, url.Values{"state": {state}, "installation_id": {"7"}, "code": {"c"}}); w.Code != 400 {
		t.Fatalf("state issued to someone else: %d", w.Code)
	}
	if w := setup(a, owner, url.Values{"state": {state}, "installation_id": {"7"}, "code": {"c"}}); w.Code != 302 ||
		w.Header().Get("Location") != "/t/"+tn.ID+"/runners/repositories" {
		t.Fatalf("setup: %d %s %s", w.Code, w.Header().Get("Location"), w.Body)
	}
	// The return page is signed with the rest: changing it breaks the state.
	if _, _, err := a.checkState(strings.Replace(state, ".runners.", ".projects.", 1), acct, time.Now()); err == nil {
		t.Fatal("a state with a changed return page was accepted")
	}
	if s := a.stateFor(ctx, tn.ID, acct, "https://evil.example", time.Now()); !strings.Contains(s, ".projects.") {
		t.Fatalf("an unknown return page is the projects page: %s", s)
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

// A signed workflow_job delivery reaches the organization's runner jobs through the one webhook route;
// an unsigned one changes nothing. The intake's rules are tested in internal/runtime/runner_job.
func TestGitHubWebhookRecordsRunnerJobs(t *testing.T) {
	a, d, tn, _ := githubAPI(t)
	ctx := context.Background()
	inst := time.Now().UnixNano()%1_000_000_000 + 1
	job := time.Now().UnixNano()%1_000_000_000 + 1
	_ = d.BindInstallation(ctx, db.GitHubInstallation{ID: inst, TenantID: tn.ID}, "")
	body := fmt.Sprintf(`{"action":"queued","installation":{"id":%d},"repository":{"id":%d,"full_name":"operator/app"},
	  "workflow_job":{"id":%d,"run_id":5,"name":"build","labels":["shippin-2x"]}}`, inst, job, job)
	send := func(sig string) int {
		r := httptest.NewRequest("POST", "/v1/github/webhook", strings.NewReader(body))
		r.Header.Set("X-Hub-Signature-256", sig)
		r.Header.Set("X-GitHub-Delivery", uuid.NewString())
		r.Header.Set("X-GitHub-Event", "workflow_job")
		w := httptest.NewRecorder()
		a.githubWebhook(w, r)
		return w.Code
	}
	count := func() (n int, status, reason string) {
		_ = d.Pool.QueryRow(ctx, `SELECT count(*), coalesce(max(status),''), coalesce(max(reason),'') FROM shippin_runner_jobs WHERE github_job_id=$1 AND tenant_id=$2`,
			job, tn.ID).Scan(&n, &status, &reason)
		return
	}
	if code := send("sha256=00"); code != 401 {
		t.Fatalf("unsigned: %d", code)
	}
	if n, _, _ := count(); n != 0 {
		t.Fatal("an unsigned delivery recorded a job")
	}
	m := hmac.New(sha256.New, []byte("whs"))
	m.Write([]byte(body))
	good := "sha256=" + hex.EncodeToString(m.Sum(nil))
	if send(good) != 204 || send(good) != 204 {
		t.Fatal("signed deliveries are acknowledged")
	}
	if n, status, reason := count(); n != 1 || status != "refused" || !strings.Contains(reason, "not assigned") {
		t.Fatalf("the job: %d %q %q, want one, refused because the repository is not assigned", n, status, reason)
	}
}
