package secret_store_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/mcpl/mcpltest"
	"shippin.cloud/console/internal/platform/secret_store"
	"shippin.cloud/console/internal/ports"
	"shippin.cloud/console/internal/servers"
)

// fakeVaultix answers the Vaultix calls the store makes, for one identity, one project and one
// environment, and keeps secrets by folder and name. Its answers follow a real Vaultix (proved in
// rig_test.go): creating a folder or a secret that exists is a 400 naming it, and a missing secret or
// folder is a 404 with a message.
type fakeVaultix struct {
	mu      sync.Mutex
	secrets map[string]string
	folders map[string]bool
	down    bool
}

func newFake(t *testing.T) (*fakeVaultix, *httptest.Server) {
	f := &fakeVaultix{secrets: map[string]string{}, folders: map[string]bool{"/": true}}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeVaultix) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		w.WriteHeader(503)
		return
	}
	var in map[string]string
	_ = json.NewDecoder(r.Body).Decode(&in)
	if r.URL.Path == "/api/v1/auth/universal-auth/login" {
		if in["clientId"] != "id-1" || in["clientSecret"] != "cs-1" {
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"message":"Invalid credentials"}`))
			return
		}
		_, _ = w.Write([]byte(`{"accessToken":"tok","expiresIn":3600}`))
		return
	}
	if r.Header.Get("Authorization") != "Bearer tok" {
		w.WriteHeader(401)
		return
	}
	q := r.URL.Query()
	project, env, path := q.Get("workspaceId")+in["workspaceId"], q.Get("environment")+in["environment"], q.Get("secretPath")+in["secretPath"]
	if project != "p1" || env != "prod" {
		w.WriteHeader(404)
		return
	}
	switch {
	case r.URL.Path == "/api/v1/folders":
		p := strings.TrimSuffix(in["path"], "/") + "/" + in["name"]
		if f.folders[p] {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"message":"Folder with name '` + in["name"] + `' already exists in path '` + in["path"] + `'","error":"BadRequest"}`))
			return
		}
		f.folders[p] = true
	case r.URL.Path == "/api/v3/secrets/raw" && r.Method == "GET":
		_, _ = w.Write([]byte(`{"secrets":[]}`))
	case strings.HasPrefix(r.URL.Path, "/api/v3/secrets/raw/"):
		k := path + "/" + strings.TrimPrefix(r.URL.Path, "/api/v3/secrets/raw/")
		if !f.folders[path] {
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"message":"Folder with path '` + path + `' in environment with slug 'prod' not found"}`))
			return
		}
		switch r.Method {
		case "POST":
			if _, ok := f.secrets[k]; ok {
				w.WriteHeader(400)
				_, _ = w.Write([]byte(`{"message":"Secret already exists","error":"BadRequest"}`))
				return
			}
			f.secrets[k] = in["secretValue"]
			_, _ = w.Write([]byte(`{"secret":{}}`))
		case "GET":
			v, ok := f.secrets[k]
			if !ok {
				w.WriteHeader(404)
				return
			}
			b, _ := json.Marshal(map[string]any{"secret": map[string]string{"secretValue": v}})
			_, _ = w.Write(b)
		case "DELETE":
			if _, ok := f.secrets[k]; !ok {
				w.WriteHeader(404)
				return
			}
			delete(f.secrets, k)
		}
	default:
		w.WriteHeader(404)
	}
}

func harness(t *testing.T) (*mcpltest.Harness, *servers.Sealer) {
	seal, err := servers.NewSealer(strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	s := mcpl.Services{}
	mcpl.Provide[ports.Sealer](s, seal)
	return mcpltest.Serve(t, actions.All, "shippin.platform.secret_store.", mcpltest.As{Actor: "ops-1", Services: s}), seal
}

func connect(url string, secret string) string {
	b, _ := json.Marshal(map[string]any{"api_url": url, "project_id": "p1", "environment": "prod", "path": "/shippin", "client_id": "id-1",
		"client_secret": secret, "reason": "Connecting the review deploy to Vaultix"})
	return string(b)
}

const path = "/v1/operator/platform/secret-store"

func TestConnectChecksFirstAndNeverReturnsTheSecret(t *testing.T) {
	h, seal := harness(t)
	fake, srv := newFake(t)
	if got := h.Call("GET", path, "", 200); got["connected"] != false {
		t.Fatalf("nothing connected yet: %v", got)
	}
	h.Call("PUT", path, connect(srv.URL, "wrong"), 400)
	h.Call("PUT", path, connect("https://user:pw@vaultix.example.com", "cs-1"), 400)
	h.Call("PUT", path, connect("http://vaultix.example.com", "cs-1"), 400) // plain http off this machine
	if got := h.Call("GET", path, "", 200); got["connected"] != false {
		t.Fatal("a connection that failed its check was saved")
	}
	got := h.Call("PUT", path, connect(srv.URL, "cs-1"), 200)
	raw, _ := json.Marshal(got)
	if got["connected"] != true || got["has_client_secret"] != true || strings.Contains(string(raw), "cs-1") || got["check_error"] != nil {
		t.Fatalf("connected: %s", raw)
	}
	if len(fake.secrets) != 0 || !fake.folders["/shippin"] {
		t.Fatalf("the check value was deleted and the folder made: %v %v", fake.secrets, fake.folders)
	}
	var sealed []byte
	_ = h.Pool.QueryRow(t.Context(), `SELECT client_secret FROM shippin_secret_store`).Scan(&sealed)
	if strings.Contains(string(sealed), "cs-1") {
		t.Fatal("the client secret is stored in the clear")
	}
	// Changing something else keeps the saved secret; a new identity needs its own.
	h.Call("PUT", path, strings.Replace(connect(srv.URL, ""), `"client_secret":"",`, "", 1), 200)
	h.Call("PUT", path, strings.Replace(connect(srv.URL, ""), `"client_id":"id-1"`, `"client_id":"id-2"`, 1), 400)

	// The port writes, reads and deletes through the saved connection.
	v := &secret_store.Vaultix{Pool: h.Pool, Sealer: seal}
	where, err := v.Where(t.Context())
	if err != nil || where.Path != "/shippin" {
		t.Fatalf("where: %+v %v", where, err)
	}
	where.Path, where.Name = "/shippin/github", "WEBHOOK_SECRET_V1"
	if err := v.Write(t.Context(), where, "whs-1"); err != nil {
		t.Fatal(err)
	}
	if got, err := v.Read(t.Context(), where); got != "whs-1" || err != nil {
		t.Fatalf("read: %q %v", got, err)
	}
	// Names are written once; a new value goes under a new name.
	if err := v.Write(t.Context(), where, "whs-2"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("a second write of one name is refused: %v", err)
	}
	if err := v.Delete(t.Context(), where); err != nil {
		t.Fatal(err)
	}
	if err := v.Delete(t.Context(), where); err != nil {
		t.Fatalf("deleting a key that is gone is fine: %v", err)
	}
}

func TestKeysHeldPinTheStore(t *testing.T) {
	h, _ := harness(t)
	fake, srv := newFake(t)
	h.Call("PUT", path, connect(srv.URL, "cs-1"), 200)
	if _, err := h.Pool.Exec(t.Context(), `INSERT INTO shippin_settings(section, vals, keys, version, updated_by)
	  VALUES ('github', '{}', '{"webhook_secret":{"project":"p1","environment":"prod","path":"/shippin/github","name":"WEBHOOK_SECRET_V1"}}', 1, 'ops-1')`); err != nil {
		t.Fatal(err)
	}
	if got := h.Call("GET", path, "", 200); got["keys_held"] != float64(1) {
		t.Fatalf("keys held: %v", got)
	}
	h.Call("PUT", path, strings.Replace(connect(srv.URL, "cs-1"), `"/shippin"`, `"/other"`, 1), 409)
	h.Call("DELETE", path, `{"reason":"Moving to another store"}`, 409)
	// A check that fails is recorded, not an error.
	fake.mu.Lock()
	fake.down = true
	fake.mu.Unlock()
	if got := h.Call("POST", path+"/test", `{"reason":"Checking the secret store"}`, 200); got["check_error"] == nil {
		t.Fatalf("the failed check is recorded: %v", got)
	}
	_, _ = h.Pool.Exec(t.Context(), `DELETE FROM shippin_settings`)
	h.Call("DELETE", path, `{"reason":"Moving to another store"}`, 204)
	if got := h.Call("GET", path, "", 200); got["connected"] != false {
		t.Fatal("disconnected")
	}
	if _, err := (&secret_store.Vaultix{Pool: h.Pool}).Where(t.Context()); err != ports.ErrNoSecretStore {
		t.Fatalf("no store: %v", err)
	}
}
