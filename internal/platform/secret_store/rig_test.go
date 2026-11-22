package secret_store_test

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"shippin.cloud/console/internal/platform/secret_store"
	"shippin.cloud/console/internal/vaultix"
)

// rig is a real Vaultix stood up by deploy/vaultix/proof-secret-store.mjs: a project with a prod
// environment, a machine identity with the project's member role, and one with no project access.
type rig struct{ url, project, env, id, secret, outsiderID, outsiderSecret string }

func rigFrom(t *testing.T) rig {
	t.Helper()
	get := func(k string) string { return os.Getenv("SHIPPIN_VAULTIX_PROOF_" + k) }
	r := rig{get("URL"), get("PROJECT"), get("ENV"), get("CLIENT_ID"), get("CLIENT_SECRET"), get("OUTSIDER_CLIENT_ID"), get("OUTSIDER_CLIENT_SECRET")}
	if r.url == "" || r.project == "" || r.env == "" || r.id == "" || r.secret == "" {
		t.Skip("no Vaultix rig: run deploy/vaultix/proof-secret-store.mjs up and set SHIPPIN_VAULTIX_PROOF_*")
	}
	return r
}

// folder is a fresh folder per run, so a rig can be used again.
func folder() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "/shippin-proof-" + hex.EncodeToString(b)
}

func wantErr(t *testing.T, err error, parts ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want an error containing %q", parts)
	}
	for _, p := range parts {
		if !strings.Contains(err.Error(), p) {
			t.Fatalf("error %q does not contain %q", err, p)
		}
	}
}

// TestRigCheck runs the connection check against a real Vaultix, and each way it fails for real.
func TestRigCheck(t *testing.T) {
	r := rigFrom(t)
	ctx := t.Context()
	f := secret_store.Fields{APIURL: r.url, ProjectID: r.project, Environment: r.env, Path: folder() + "/panel", ClientID: r.id}
	if err := secret_store.Check(ctx, f, r.secret); err != nil {
		t.Fatalf("check: %v", err)
	}
	// Again on the same folder: the folders exist now, which counts as made.
	if err := secret_store.Check(ctx, f, r.secret); err != nil {
		t.Fatalf("second check on an existing folder: %v", err)
	}
	// The check value is gone: the folder lists nothing.
	c := vaultix.New(r.url, r.id, r.secret, 10*time.Second)
	if items, err := c.List(ctx, vaultix.Scope{Project: r.project, Env: r.env, Path: f.Path}); err != nil || len(items) != 0 {
		t.Fatalf("the check value is deleted: %v %v", items, err)
	}

	wantErr(t, secret_store.Check(ctx, f, "not-the-secret"), "signing in as the machine identity failed", "401", "Invalid credentials")
	bad := f
	bad.ClientID = "00000000-0000-0000-0000-000000000000"
	wantErr(t, secret_store.Check(ctx, bad, r.secret), "signing in as the machine identity failed", "401")

	if r.outsiderID == "" {
		t.Fatal("set SHIPPIN_VAULTIX_PROOF_OUTSIDER_CLIENT_ID and _SECRET for the identity without project access")
	}
	out := f
	out.ClientID = r.outsiderID
	wantErr(t, secret_store.Check(ctx, out, r.outsiderSecret), "signed in but can't read project "+r.project, "give it access to the project", "403", "not a member of this project")

	env := f
	env.Environment = "no-such-env"
	wantErr(t, secret_store.Check(ctx, env, r.secret), "can't read project", "environment 'no-such-env' was not found")
	proj := f
	proj.ProjectID = "00000000-0000-4000-8000-000000000000"
	wantErr(t, secret_store.Check(ctx, proj, r.secret), "can't read project", "not found")
}

// TestRigSecretStore connects the panel through its update action and uses the saved connection as
// ports.Secrets: write, read, a refused second write, delete, and deleting what is already gone.
func TestRigSecretStore(t *testing.T) {
	r := rigFrom(t)
	h, seal := harness(t)
	body := func(id, secret string) string {
		b, _ := json.Marshal(map[string]any{"api_url": r.url, "project_id": r.project, "environment": r.env, "path": folder(), "client_id": id,
			"client_secret": secret, "reason": "Proving the secret store against a real Vaultix"})
		return string(b)
	}
	got := h.Call("PUT", path, body(r.id, "not-the-secret"), 400)
	if msg, _ := json.Marshal(got); !strings.Contains(string(msg), "signing in as the machine identity failed") {
		t.Fatalf("wrong secret: %s", msg)
	}
	if r.outsiderID != "" {
		got = h.Call("PUT", path, body(r.outsiderID, r.outsiderSecret), 400)
		if msg, _ := json.Marshal(got); !strings.Contains(string(msg), "give it access to the project") {
			t.Fatalf("no project access: %s", msg)
		}
	}
	if got := h.Call("GET", path, "", 200); got["connected"] != false {
		t.Fatal("a connection that failed its check was saved")
	}
	got = h.Call("PUT", path, body(r.id, r.secret), 200)
	raw, _ := json.Marshal(got)
	if got["connected"] != true || got["check_error"] != nil || strings.Contains(string(raw), r.secret) {
		t.Fatalf("connected: %s", raw)
	}
	if got := h.Call("POST", path+"/test", `{"reason":"Checking the secret store"}`, 200); got["check_error"] != nil {
		t.Fatalf("test: %v", got)
	}

	ctx := t.Context()
	v := &secret_store.Vaultix{Pool: h.Pool, Sealer: seal}
	where, err := v.Where(ctx)
	if err != nil {
		t.Fatal(err)
	}
	where.Path, where.Name = where.Path+"/github", "WEBHOOK_SECRET_V1"
	if err := v.Write(ctx, where, "whs-1"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got, err := v.Read(ctx, where); got != "whs-1" || err != nil {
		t.Fatalf("read: %q %v", got, err)
	}
	// Names are written once: a second create of the same name is refused and the value kept.
	wantErr(t, v.Write(ctx, where, "whs-2"), "400", "Secret already exists")
	if got, err := v.Read(ctx, where); got != "whs-1" || err != nil {
		t.Fatalf("read after the refused write: %q %v", got, err)
	}
	if err := v.Delete(ctx, where); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := v.Read(ctx, where); !errors.Is(err, vaultix.ErrNotFound) {
		t.Fatalf("read after delete is not found: %v", err)
	}
	if err := v.Delete(ctx, where); err != nil {
		t.Fatalf("deleting a key that is gone is fine: %v", err)
	}
	gone := where
	gone.Path = where.Path + "/never-made"
	if err := v.Delete(ctx, gone); err != nil {
		t.Fatalf("deleting in a folder that was never made is fine: %v", err)
	}
	// A second write in an existing folder works: EnsureFolder takes "already exists" as made.
	where.Name = "WEBHOOK_SECRET_V2"
	if err := v.Write(ctx, where, "whs-2"); err != nil {
		t.Fatalf("write in an existing folder: %v", err)
	}
	if err := v.Delete(ctx, where); err != nil {
		t.Fatal(err)
	}
}
