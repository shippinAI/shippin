package setting_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/integrations/github_app"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/mcpl/mcpltest"
	"shippin.cloud/console/internal/ports"
	"shippin.cloud/console/internal/settings"
)

// memSecrets is a secret store in memory that remembers what was written and deleted.
type memSecrets struct {
	mu      sync.Mutex
	keys    map[ports.SecretRef]string
	deleted []string
	fail    error
}

func (m *memSecrets) Where(context.Context) (ports.SecretRef, error) {
	return ports.SecretRef{Project: "p1", Environment: "prod", Path: "/shippin"}, nil
}
func (m *memSecrets) Write(_ context.Context, r ports.SecretRef, v string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail != nil {
		return m.fail
	}
	if _, ok := m.keys[r]; ok {
		return errors.New("a key name is written once")
	}
	m.keys[r] = v
	return nil
}
func (m *memSecrets) Read(_ context.Context, r ports.SecretRef) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.keys[r]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}
func (m *memSecrets) Delete(_ context.Context, r ports.SecretRef) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.keys, r)
	m.deleted = append(m.deleted, r.Path+"/"+r.Name)
	return nil
}
func (m *memSecrets) names() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for r := range m.keys {
		out = append(out, r.Path+"/"+r.Name)
	}
	return out
}

func file() *config.Config {
	return &config.Config{
		Jobs: config.Jobs{Workers: 2, MaxAttempts: 5, LeaseSeconds: 60, HeartbeatSeconds: 20, PollSeconds: 2, BackoffSeconds: 5,
			BackoffMaxSecs: 300, ReconcileSeconds: 60, DriftSeconds: 300, KeepDays: 14, IdempotencyHours: 24},
		GitHub: config.GitHub{AppSlug: "shippin", APIBase: "https://api.github.com", WebBase: "https://github.com", RefreshMinutes: 45,
			PollMinutes: 10, StateMinutes: 15, MaxRepos: 100, TokenPermissions: map[string]string{"contents": "write"},
			ClientID: "Iv1.file", ClientSecret: "env-client-secret"},
	}
}

func pemKey(t *testing.T) string {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}))
}

func setup(t *testing.T) (*mcpltest.Harness, *settings.Store, *memSecrets) {
	h := mcpltest.Serve(t, actions.All, "shippin.platform.setting.", mcpltest.As{Actor: "ops-1", Services: mcpl.Services{}})
	h.React(actions.Reactions)
	mem := &memSecrets{keys: map[ports.SecretRef]string{}}
	var store ports.Secrets
	st, err := settings.New(h.Pool, file(), actions.Settings, func() ports.Secrets { return store })
	if err != nil {
		t.Fatal(err)
	}
	mcpl.Provide[*settings.Store](h.As.Services, st)
	t.Cleanup(func() { store = nil })
	connect := func() { store = mem }
	connect()
	return h, st, mem
}

func body(v map[string]any) string {
	v["reason"] = "Setting up the review deploy"
	b, _ := json.Marshal(v)
	return string(b)
}

func field(view map[string]any, name string) map[string]any {
	for _, f := range view["fields"].([]any) {
		if m := f.(map[string]any); m["name"] == name {
			return m
		}
	}
	return nil
}

const base = "/v1/operator/platform/settings"

func TestSectionsComeFromTheFileUntilSaved(t *testing.T) {
	h, st, _ := setup(t)
	items := h.Call("GET", base, "", 200)["items"].([]any)
	if len(items) != len(actions.Settings) {
		t.Fatalf("every section is listed: %d of %d", len(items), len(actions.Settings))
	}
	jobs := h.Call("GET", base+"/jobs", "", 200)
	if jobs["source"] != "file" || jobs["version"] != float64(0) || field(jobs, "workers")["value"] != float64(2) {
		t.Fatalf("jobs from the file: %v", jobs)
	}
	if k := field(h.Call("GET", base+"/github", "", 200), "client_secret")["key"].(map[string]any); k["in"] != "environment" || k["saved"] != true {
		t.Fatalf("a key still in the environment says so: %v", k)
	}
	if v, _ := st.Key(t.Context(), "github", "client_secret"); v != "env-client-secret" {
		t.Fatal("an unsaved section's key is read from the environment")
	}
	h.Call("GET", base+"/nope", "", 404)
}

func TestSaveValidatesAuditsAndRefusesStaleVersions(t *testing.T) {
	h, st, _ := setup(t)
	h.Call("PUT", base+"/jobs", body(map[string]any{"version": 0, "values": map[string]any{"workers": 0}}), 400)
	h.Call("PUT", base+"/jobs", body(map[string]any{"version": 0, "values": map[string]any{"nope": 1}}), 400)
	h.Call("PUT", base+"/jobs", body(map[string]any{"version": 0, "values": map[string]any{"enabled": true, "heartbeat_seconds": 40}}), 400) // over half the lease
	h.Call("PUT", base+"/jobs", `{"version":0,"values":{"enabled":true}}`, 400)                                           // no reason
	h.Call("PUT", base+"/runners", body(map[string]any{"version": 0, "values": map[string]any{"enabled": true, "fleet": "01a14149-0000-7000-8000-00000000000a", "job_limit_minutes": 60}}), 400)

	saved := h.Call("PUT", base+"/jobs", body(map[string]any{"version": 0, "values": map[string]any{"enabled": true, "workers": 4}}), 200)
	if saved["source"] != "panel" || saved["version"] != float64(1) || field(saved, "workers")["value"] != float64(4) || field(saved, "workers")["file_value"] != float64(2) {
		t.Fatalf("saved: %v", saved)
	}
	h.Call("PUT", base+"/jobs", body(map[string]any{"version": 0, "values": map[string]any{"workers": 3}}), 409)
	if j, v := settings.Get[config.Jobs](t.Context(), st, "jobs"); !j.Enabled || j.Workers != 4 || j.LeaseSeconds != 60 || v != 1 {
		t.Fatalf("readers see the saved section at once: %+v v%d", j, v)
	}
	// Runners need the job queue, and the queue can't stop under them.
	h.Call("PUT", base+"/runners", body(map[string]any{"version": 0, "values": map[string]any{"enabled": true, "fleet": "01a14149-0000-7000-8000-00000000000a", "job_limit_minutes": 60}}), 200)
	h.Call("PUT", base+"/jobs", body(map[string]any{"version": 1, "values": map[string]any{"enabled": false}}), 400)

	versions := h.Call("GET", base+"/jobs/versions", "", 200)["items"].([]any)
	if v := versions[0].(map[string]any); len(versions) != 1 || v["reason"] != "Setting up the review deploy" || v["changed_by"] != "ops-1" {
		t.Fatalf("history: %v", versions)
	}
}

func TestKeysGoToTheSecretStoreOnly(t *testing.T) {
	h, st, mem := setup(t)
	key := pemKey(t)
	h.Call("PUT", base+"/github", body(map[string]any{"version": 0, "keys": map[string]any{"private_key": "not a pem"}}), 400)
	h.Call("PUT", base+"/github", body(map[string]any{"version": 0, "keys": map[string]any{"client_secret": "two\nlines"}}), 400)
	// Turning the App on needs every key.
	h.Call("PUT", base+"/github", body(map[string]any{"version": 0, "values": map[string]any{"enabled": true}, "keys": map[string]any{"private_key": key}}), 400)

	saved := h.Call("PUT", base+"/github", body(map[string]any{"version": 0, "values": map[string]any{"enabled": true, "client_id": "Iv1.panel"},
		"keys": map[string]any{"private_key": key, "webhook_secret": "whs-1"}, "import": []string{"client_secret"}}), 200)
	raw, _ := json.Marshal(saved)
	for _, secret := range []string{"BEGIN RSA", "whs-1", "env-client-secret"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("the response carries a key: %s", secret)
		}
	}
	if k := field(saved, "private_key")["key"].(map[string]any); k["in"] != "secret_store" || k["name"] != "/shippin/github/PRIVATE_KEY_V1" {
		t.Fatalf("where the key is: %v", k)
	}
	var row string
	_ = h.Pool.QueryRow(t.Context(), `SELECT s.vals::text || s.keys::text || string_agg(v.vals::text, '') FROM shippin_settings s JOIN shippin_setting_versions v USING (section) WHERE section='github' GROUP BY s.vals, s.keys`).Scan(&row)
	for _, secret := range []string{"BEGIN RSA", "whs-1", "env-client-secret"} {
		if strings.Contains(row, secret) {
			t.Fatalf("the database holds a key: %s", secret)
		}
	}
	if got := mem.names(); len(got) != 3 {
		t.Fatalf("three keys in the store: %v", got)
	}
	if v, _ := st.Key(t.Context(), "github", "client_secret"); v != "env-client-secret" {
		t.Fatal("the imported key is read from the store")
	}
	app, _ := settings.Get[github_app.App](t.Context(), st, "github")
	if !app.Enabled || app.ClientID != "Iv1.panel" || app.AppSlug != "shippin" {
		t.Fatalf("the section as its users read it: %+v", app)
	}

	// Replacing a key writes a new name; the reaction deletes the old one after the commit.
	h.Call("PUT", base+"/github", body(map[string]any{"version": 1, "keys": map[string]any{"webhook_secret": "whs-2"}}), 200)
	if v, _ := st.Key(t.Context(), "github", "webhook_secret"); v != "whs-2" {
		t.Fatalf("the new key is in force: %q", v)
	}
	if fmt.Sprint(mem.deleted) != "[/shippin/github/WEBHOOK_SECRET_V1]" {
		t.Fatalf("the replaced key was deleted: %v", mem.deleted)
	}
	var queued int
	_ = h.Pool.QueryRow(t.Context(), `SELECT count(*) FROM shippin_setting_retired_secrets`).Scan(&queued)
	if queued != 0 {
		t.Fatalf("nothing is left to delete: %d", queued)
	}
	// A key the App needs can't be removed while it is on.
	h.Call("PUT", base+"/github", body(map[string]any{"version": 2, "keys": map[string]any{"webhook_secret": nil}}), 400)
}

func TestAFailedWriteSavesNothingAndIsCleanedUp(t *testing.T) {
	h, st, mem := setup(t)
	mem.fail = errors.New("vaultix is down")
	h.Call("PUT", base+"/github", body(map[string]any{"version": 0, "keys": map[string]any{"webhook_secret": "whs-1"}}), 502)
	if _, v := settings.Get[github_app.App](t.Context(), st, "github"); v != 0 {
		t.Fatal("a failed save saved the section")
	}
	// The name it tried to write stays queued, due later, so the sweep removes it if it was written.
	var due bool
	_ = h.Pool.QueryRow(t.Context(), `SELECT due_at > now() FROM shippin_setting_retired_secrets WHERE ref->>'name' = 'WEBHOOK_SECRET_V1'`).Scan(&due)
	if !due {
		t.Fatal("the attempted key is queued for removal later")
	}
	mem.fail = nil
	_, _ = h.Pool.Exec(t.Context(), `UPDATE shippin_setting_retired_secrets SET due_at = now()`)
	if n, err := st.Sweep(t.Context()); n != 1 || err != nil {
		t.Fatalf("sweep: %d %v", n, err)
	}
}

func TestNoSecretStoreMeansNoKeys(t *testing.T) {
	h, st, _ := setup(t)
	st2, _ := settings.New(h.Pool, file(), actions.Settings, func() ports.Secrets { return nil })
	mcpl.Provide[*settings.Store](h.As.Services, st2)
	h.Call("PUT", base+"/github", body(map[string]any{"version": 0, "keys": map[string]any{"webhook_secret": "whs-1"}}), 409)
	// Settings without keys still save.
	h.Call("PUT", base+"/jobs", body(map[string]any{"version": 0, "values": map[string]any{"workers": 3}}), 200)
	_ = st
}
