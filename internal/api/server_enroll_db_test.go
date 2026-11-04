package api

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"shippin.cloud/console/internal/enroll"
)

// TestEnrollEndToEnd: the real hostd agent (internal/enroll) against the real console handlers and
// database: token → enroll → active → beats stop → stale → beats resume → active → revoke → the
// agent is rejected and drops its credential. The token works exactly once.
func TestEnrollEndToEnd(t *testing.T) {
	_, _, tn, srv := serverTestAPI(t, "org_admin")
	base := "/v1/tenants/" + tn.ID + "/servers"
	_, g, _ := srvCall(t, srv, "POST", base, `{"slug":"edge","label":"Edge","site":"edge"}`, "")
	id := g["id"].(string)
	_, tok, _ := srvCall(t, srv, "POST", base+"/"+id+"/enroll-token", ``, "")
	dir := t.TempDir()
	cfg := enroll.Config{ConsoleURL: srv.URL, TokenFile: filepath.Join(dir, "token"), CredentialFile: filepath.Join(dir, "cred")}
	if err := cfg.Check(); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(cfg.TokenFile, []byte(tok["token"].(string)), 0o600)
	agent := &enroll.Agent{Cfg: cfg, Version: "test-1", Hostname: func() (string, error) { return "edge-test", nil }}
	if st := stateOf(t, srv, tn.ID, id); st != "pending" {
		t.Fatalf("before enroll: %s", st)
	}
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- agent.Run(ctx) }()
	waitState(t, srv, tn.ID, id, "active")
	if c, _, _ := srvCall(t, srv, "POST", "/v1/servers/enroll", `{"token":"`+tok["token"].(string)+`"}`, ""); c != 401 {
		t.Fatalf("second redemption of the token: %d", c)
	}
	stop()
	<-done
	waitState(t, srv, tn.ID, id, "stale") // heartbeat 1s × missed 1
	ctx, stop = context.WithCancel(context.Background())
	defer stop()
	go func() { done <- agent.Run(ctx) }() // restarts from the saved credential, no token needed
	waitState(t, srv, tn.ID, id, "active")
	if c, _, _ := srvCall(t, srv, "POST", base+"/"+id+"/revoke", ``, ""); c != 200 {
		t.Fatalf("revoke: %d", c)
	}
	select {
	case err := <-done:
		if err != enroll.ErrRejected {
			t.Fatalf("agent after revoke: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("agent kept beating after revoke")
	}
	if _, err := os.Stat(cfg.CredentialFile); !os.IsNotExist(err) {
		t.Fatal("revoked credential left on disk")
	}
	if c, _, _ := srvCall(t, srv, "POST", "/v1/servers/heartbeat", `{}`, "shph_bogus"); c != 401 {
		t.Fatalf("garbage credential: %d", c)
	}
}

func waitState(t *testing.T, s *httptest.Server, tid, id, want string) {
	t.Helper()
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if stateOf(t, s, tid, id) == want {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("server never reached %s (now %s)", want, stateOf(t, s, tid, id))
}
