package api

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/jobs"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/ports"
	"shippin.cloud/console/internal/servers"
	"shippin.cloud/console/internal/settings"
)

// liveSecrets is a secret store in memory; connected is false to act as one that can't be reached.
type liveSecrets struct {
	mu        sync.Mutex
	keys      map[ports.SecretRef]string
	connected bool
}

func (m *liveSecrets) Where(context.Context) (ports.SecretRef, error) {
	return ports.SecretRef{Project: "p1", Environment: "prod", Path: "/shippin"}, nil
}
func (m *liveSecrets) Write(_ context.Context, r ports.SecretRef, v string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keys[r] = v
	return nil
}
func (m *liveSecrets) Read(_ context.Context, r ports.SecretRef) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.keys[r]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}
func (m *liveSecrets) Delete(_ context.Context, r ports.SecretRef) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.keys, r)
	return nil
}

// liveAPI is an API whose sections are read from the settings table, with keys in mem. The settings
// tables are shared by every run of this database, so they start and end empty.
func liveAPI(t *testing.T) (*API, *db.DB, db.Tenant, *liveSecrets) {
	a, d, tn := notifyAPI(t)
	a.jobs = jobs.NewStore(d.Pool)
	clear := func() {
		if _, err := d.Pool.Exec(context.Background(), `DELETE FROM shippin_setting_versions; DELETE FROM shippin_settings;
		  DELETE FROM shippin_setting_retired_secrets`); err != nil {
			t.Fatal(err)
		}
	}
	clear()
	t.Cleanup(clear)
	mem := &liveSecrets{keys: map[ports.SecretRef]string{}, connected: true}
	st, err := settings.New(d.Pool, a.cfg, actions.Settings, func() ports.Secrets {
		if !mem.connected {
			return nil
		}
		return mem
	})
	if err != nil {
		t.Fatal(err)
	}
	a.settings = st
	return a, d, tn, mem
}

// save saves a section as shippin.platform.setting.update does, and publishes its event when told to,
// as the action does once the change has committed.
func save(t *testing.T, a *API, section string, values map[string]any, keys map[string]string, publish bool) {
	t.Helper()
	ctx := context.Background()
	_, version, err := a.settings.Values(ctx, section)
	if err != nil {
		t.Fatal(err)
	}
	ch := settings.Change{Section: section, Base: version, Values: map[string]json.RawMessage{}, Keys: map[string]*string{}, By: "ops-1", Reason: "Trying the live settings"}
	for k, v := range values {
		ch.Values[k], _ = json.Marshal(v)
	}
	for k, v := range keys {
		ch.Keys[k] = &v
	}
	tx, err := a.db.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := a.settings.Save(ctx, tx, ch); err != nil {
		t.Fatalf("save %s: %v", section, err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if publish {
		a.events.Publish(mcpl.Event{ID: settingUpdated, Action: "shippin.platform.setting.update", At: time.Now()})
	}
}

func eventually(t *testing.T, within time.Duration, what string, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(within); time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatalf("not within %s: %s", within, what)
}

// The event the job queue follows is the one a save emits.
func TestSettingUpdatedIsTheSaveEvent(t *testing.T) {
	for _, r := range actions.Reactions {
		if slices.Contains(r.On, settingUpdated) {
			return
		}
	}
	t.Fatalf("no reaction follows %s: the event a settings save emits has another name", settingUpdated)
}

// Saving the job queue section starts, restarts and stops the runner without a restart (CFG-01).
func TestJobQueueFollowsItsSection(t *testing.T) {
	a, d, tn, _ := liveAPI(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	// Hold the reconciler's leader lock, so its passes here leave other tests' rows alone.
	held, release, err := d.TryLeader(ctx, reconcileLock)
	if err != nil || !held {
		t.Fatalf("leader lock: %v %v", held, err)
	}
	t.Cleanup(release)
	if _, err := d.Pool.Exec(ctx, `DELETE FROM jobs WHERE status IN ('queued','running')`); err != nil {
		t.Fatal(err)
	}
	a.cfg.Jobs = testJobsCfg()
	a.cfg.Jobs.Enabled = false // the file has the queue off
	a.StartJobs(ctx)
	if a.jobRunner() != nil {
		t.Fatal("the file has the job queue off")
	}
	// A runner.remove of a machine that doesn't exist succeeds at once and touches nothing else.
	enqueue := func() string {
		j, _, err := a.jobs.Enqueue(ctx, jobs.Job{TenantID: tn.ID, Kind: jobs.KindRunnerRemove, TargetType: "runner_machine",
			TargetID: uuid.NewString(), MaxAttempts: 3, CreatedBy: runnerActor})
		if err != nil {
			t.Fatal(err)
		}
		if r := a.jobRunner(); r != nil {
			r.Wake()
		}
		return j.ID
	}
	status := func(id string) string {
		j, _ := a.jobs.Get(ctx, id)
		if j == nil {
			return ""
		}
		return j.Status
	}
	first := enqueue()
	time.Sleep(300 * time.Millisecond)
	if s := status(first); s != jobs.Queued {
		t.Fatalf("with the queue off a job waits: %s", s)
	}

	save(t, a, "jobs", map[string]any{"enabled": true}, nil, true)
	eventually(t, 5*time.Second, "turning the queue on starts a runner that claims work", func() bool {
		return a.jobRunner() != nil && status(first) == jobs.Succeeded
	})
	started := a.jobRunner()

	// What is read where it is used changes nothing that runs.
	save(t, a, "jobs", map[string]any{"max_attempts": 4}, nil, true)
	a.applyJobs(ctx)
	if a.jobRunner() != started || a.liveJobs(ctx).MaxAttempts != 4 {
		t.Fatalf("a change to tries restarted the runner, or isn't read: %d", a.liveJobs(ctx).MaxAttempts)
	}
	// A timing restarts it.
	save(t, a, "jobs", map[string]any{"poll_seconds": 2}, nil, true)
	eventually(t, 5*time.Second, "a timing change restarts the runner", func() bool {
		r := a.jobRunner()
		return r != nil && r != started
	})
	second := enqueue()
	eventually(t, 5*time.Second, "the restarted runner claims work", func() bool { return status(second) == jobs.Succeeded })

	// Saved in another panel (no event here): seen within settings.Fresh, and the queue stops.
	save(t, a, "jobs", map[string]any{"enabled": false}, nil, false)
	eventually(t, settings.Fresh+5*time.Second, "turning the queue off stops the runner", func() bool { return a.jobRunner() == nil })
	third := enqueue()
	time.Sleep(2500 * time.Millisecond)
	if s := status(third); s != jobs.Queued {
		t.Fatalf("a stopped queue claimed work: %s", s)
	}
	_, _ = d.Pool.Exec(context.Background(), `DELETE FROM jobs WHERE id=$1`, third)
}

func signed(secret, body string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

func deliver(a *API, secret string) int {
	body := `{"zen":"Keep it logically awesome."}`
	r := httptest.NewRequest("POST", "/v1/github/webhook", strings.NewReader(body))
	r.Header.Set("X-Hub-Signature-256", signed(secret, body))
	r.Header.Set("X-GitHub-Delivery", uuid.NewString())
	r.Header.Set("X-GitHub-Event", "ping")
	w := httptest.NewRecorder()
	a.githubWebhook(w, r)
	return w.Code
}

// verifiesWith reports whether the App signs its JWT with key's private half.
func verifiesWith(t *testing.T, a *API, key *rsa.PrivateKey) bool {
	t.Helper()
	app, _ := a.github(context.Background())
	if app == nil {
		return false
	}
	jwt, err := app.JWT()
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(jwt, ".")
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	return rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, h[:], sig) == nil
}

// The GitHub App and build runners follow their sections: the App is built from the keys in the
// secret store, rebuilt when the section changes and only then, and off when the section is.
func TestGitHubAndRunnersFollowTheirSections(t *testing.T) {
	a, d, tn, mem := liveAPI(t)
	ctx := context.Background()
	a.cfg.Jobs = testJobsCfg() // runners need the queue on
	a.runner.Store(jobs.NewRunner(a.jobs, jobs.Settings{}))
	a.cfg.GitHub = config.GitHub{AppSlug: "shippin", APIBase: "https://api.github.test", WebBase: "https://github.test",
		RefreshMinutes: 45, PollMinutes: 10, StateMinutes: 15, MaxRepos: 100, TokenPermissions: map[string]string{"contents": "write"},
		WebhookSecret: "whs-env"}
	owner := &tenantView{Tenant: tn, Roles: []string{"org_owner"}}
	off := func(when string) {
		t.Helper()
		if app, _ := a.github(ctx); app != nil {
			t.Fatalf("%s: the App is on", when)
		}
		if code := deliver(a, "whs-env"); code != 404 {
			t.Fatalf("%s: the webhook answers %d", when, code)
		}
		w := httptest.NewRecorder()
		if _, _, ok := a.githubFor(w, httptest.NewRequest("GET", "/x", nil), owner); ok || w.Code != 404 {
			t.Fatalf("%s: a tenant's GitHub routes answer %d", when, w.Code)
		}
		if _, err := mcpl.Need[ports.GitHub](&mcpl.Call{Ctx: ctx, Services: a.actionServices()}); err == nil {
			t.Fatalf("%s: actions get a GitHub port", when)
		}
		if a.runnersOn(ctx) {
			t.Fatalf("%s: runners start machines without the App", when)
		}
	}
	off("the file has GitHub off")

	k1, _ := rsa.GenerateKey(rand.Reader, 2048)
	pem1 := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k1)}))
	save(t, a, "github", map[string]any{"enabled": true, "client_id": "Iv1.panel", "max_repos": 50},
		map[string]string{"client_secret": "cs-1", "private_key": pem1, "webhook_secret": "whs-1"}, true)
	app, g := a.github(ctx)
	if app == nil || app.ClientID != "Iv1.panel" || app.ClientSecret != "cs-1" || g.MaxRepos != 50 || !verifiesWith(t, a, k1) {
		t.Fatalf("the App is built from the section and the store's keys: %v", app != nil)
	}
	if len(mem.keys) != 3 {
		t.Fatalf("the keys are in the store: %d", len(mem.keys))
	}
	if code := deliver(a, "whs-1"); code != 204 {
		t.Fatalf("a delivery signed with the saved secret: %d", code)
	}
	if code := deliver(a, "whs-env"); code != 401 {
		t.Fatalf("a delivery signed with the environment's secret: %d", code)
	}
	if _, err := mcpl.Need[ports.GitHub](&mcpl.Call{Ctx: ctx, Services: a.actionServices()}); err != nil {
		t.Fatalf("actions get the GitHub port once it is on: %v", err)
	}
	// Unchanged, it is the same client (its installation tokens kept), even with the store out of reach.
	mem.connected = false
	a.settings.Invalidate()
	if again, _ := a.github(ctx); again != app {
		t.Fatal("an unchanged section rebuilt the App")
	}
	mem.connected = true

	// A new webhook secret: the new one is accepted, the old refused.
	save(t, a, "github", nil, map[string]string{"webhook_secret": "whs-2"}, true)
	if code := deliver(a, "whs-2"); code != 204 {
		t.Fatalf("a delivery signed with the new secret: %d", code)
	}
	if code := deliver(a, "whs-1"); code != 401 {
		t.Fatalf("a delivery signed with the replaced secret: %d", code)
	}
	if again, _ := a.github(ctx); again == app || !verifiesWith(t, a, k1) {
		t.Fatal("a changed section builds the App again, from the same private key")
	}

	// Build runners: the section decides the hosts, the time limit, the storage and whether runners run.
	fleet, host := uuid.NewString(), uuid.NewString()
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO infra_fleets(id,label,created_by) VALUES($1::uuid,'Runners','test')`, []any{fleet}},
		{`INSERT INTO servers(id,tenant_id,slug,label,site,state,created_by,hv_kind,fleet_id) VALUES
		    ($1::uuid,$2::uuid,'r-'||$1::text,'Runner host','main','planned','test','incus',$3::uuid)`, []any{host, servers.SystemTenantID, fleet}},
	} {
		if _, err := d.Pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("%s: %v", q.sql, err)
		}
	}
	if hs, _ := (machinePort{a}).Hosts(ctx); len(hs) != 0 || a.runnersOn(ctx) {
		t.Fatalf("the file has runners off: %v", hs)
	}
	save(t, a, "runners", map[string]any{"enabled": true, "fleet": fleet, "storage": "fast", "job_limit_minutes": 30}, nil, true)
	if hs, err := (machinePort{a}).Hosts(ctx); err != nil || len(hs) != 1 || hs[0] != host {
		t.Fatalf("runner hosts are the saved fleet's: %v %v", hs, err)
	}
	if !a.runnersOn(ctx) || a.runnerDeps(ctx).JobLimit != 30*time.Minute || a.liveRunners(ctx).Storage != "fast" {
		t.Fatalf("dispatch and the sweep read the saved section: on %v, limit %s", a.runnersOn(ctx), a.runnerDeps(ctx).JobLimit)
	}
	save(t, a, "runners", map[string]any{"job_limit_minutes": 90}, nil, true)
	if a.runnerDeps(ctx).JobLimit != 90*time.Minute {
		t.Fatalf("a new time limit: %s", a.runnerDeps(ctx).JobLimit)
	}
	save(t, a, "runners", map[string]any{"enabled": false}, nil, true)
	if hs, _ := (machinePort{a}).Hosts(ctx); len(hs) != 0 || a.runnersOn(ctx) {
		t.Fatalf("runners turned off: %v", hs)
	}

	// GitHub turned off in the panel: every GitHub route is off, as when the file turns it off.
	save(t, a, "github", map[string]any{"enabled": false}, nil, true)
	off("the panel turned GitHub off")
	// A section the store can't give the keys for leaves GitHub off rather than half on.
	save(t, a, "github", map[string]any{"enabled": true}, nil, true)
	if app, _ := a.github(ctx); app == nil {
		t.Fatal("turned on again with its keys saved")
	}
	mem.connected = false
	save(t, a, "github", map[string]any{"max_repos": 60}, nil, true)
	if app, _ := a.github(ctx); app != nil {
		t.Fatal("a changed section whose keys can't be read left the App on")
	}
	mem.connected = true
}
