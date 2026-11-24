package enroll

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestConfigCheck(t *testing.T) {
	var off *Config
	if off.Check() != nil {
		t.Fatal("nil = off")
	}
	for u, ok := range map[string]bool{"https://console.example": true, "http://127.0.0.1:9": true,
		"http://console.example": false, "https://console.example/v1": false, "": false} {
		c := &Config{ConsoleURL: u, TokenFile: "/a", CredentialFile: "/b"}
		if (c.Check() == nil) != ok {
			t.Errorf("%q: want ok=%v", u, ok)
		}
	}
	if (&Config{ConsoleURL: "https://x", TokenFile: "rel", CredentialFile: "/b"}).Check() == nil {
		t.Error("relative path accepted")
	}
}

// TestAgentAgainstFake: enroll once, token file removed, beats carry the bearer, a 401 removes
// the credential and stops the loop; errors never contain a secret.
func TestAgentAgainstFake(t *testing.T) {
	var beats atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/servers/enroll":
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in["token"] != "shpe_x" || in["hostname"] != "edge" {
				w.WriteHeader(401)
				return
			}
			_, _ = w.Write([]byte(`{"server_id":"s1","credential":"shph_y","heartbeat_seconds":1}`))
		case "/v1/servers/heartbeat":
			if r.Header.Get("Authorization") != "Bearer shph_y" || beats.Add(1) > 2 {
				w.WriteHeader(401)
				return
			}
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	cfg := Config{ConsoleURL: srv.URL, TokenFile: filepath.Join(dir, "token"), CredentialFile: filepath.Join(dir, "cred")}
	_ = os.WriteFile(cfg.TokenFile, []byte("shpe_x\n"), 0o600)
	a := &Agent{Cfg: cfg, Version: "t", Hostname: func() (string, error) { return "edge", nil }}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.Run(ctx); err != ErrRejected || beats.Load() != 3 {
		t.Fatalf("run: %v after %d beats", err, beats.Load())
	}
	if _, err := os.Stat(cfg.TokenFile); !os.IsNotExist(err) {
		t.Fatal("the token file must be removed after enrollment")
	}
	if _, err := os.Stat(cfg.CredentialFile); !os.IsNotExist(err) {
		t.Fatal("a rejected credential must be removed")
	}
	if _, err := a.Enroll(ctx); err == nil || strings.Contains(err.Error(), "shpe_") {
		t.Fatalf("no token, no credential: %v", err)
	}
}

func TestCredentialFileMode(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c")
	if err := writeSecret(p, saved{Credential: "shph_z", HeartbeatSeconds: 15}); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", st.Mode().Perm())
	}
}

func TestHeartbeatDoesNotFollowRedirectWithHostCredential(t *testing.T) {
	var redirected atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected.Add(1)
	}))
	defer target.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer shph_secret" {
			t.Errorf("source did not receive host credential")
		}
		http.Redirect(w, r, target.URL+"/capture", http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	a := &Agent{Cfg: Config{ConsoleURL: source.URL}}
	if err := a.Beat(context.Background(), &saved{Credential: "shph_secret"}); err == nil || !strings.Contains(err.Error(), "HTTP 307") {
		t.Fatalf("redirect should be reported as an endpoint failure, got %v", err)
	}
	if redirected.Load() != 0 {
		t.Fatalf("host credential followed redirect to another endpoint %d times", redirected.Load())
	}
}

func TestHeartbeatExecutesTypedCommandAndSubmitsResult(t *testing.T) {
	var resultRequests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer shph_command" {
			t.Error("credential missing from authenticated request")
		}
		switch r.URL.Path {
		case "/v1/servers/heartbeat":
			if err := json.NewEncoder(w).Encode(map[string]Command{"command": {
				ID: "2d6a4a79-9c0f-46df-83da-8ec9cde05a11", Type: "probe_incus_application",
				DeploymentID: "2d6a4a79-9c0f-46df-83da-8ec9cde05a11", Generation: 2,
				PlanSHA256: strings.Repeat("a", 64), IdempotencyKey: strings.Repeat("b", 64),
				Payload: json.RawMessage(`{}`), ExpiresAt: time.Now().Add(time.Minute),
			}}); err != nil {
				t.Errorf("encode heartbeat command: %v", err)
			}
		case "/v1/servers/commands/2d6a4a79-9c0f-46df-83da-8ec9cde05a11/result":
			var got CommandResult
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			if got.Status != "passing" || got.DurationMS != 17 {
				t.Errorf("result payload = %#v", got)
			}
			resultRequests.Add(1)
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()
	a := &Agent{Cfg: Config{ConsoleURL: srv.URL}, HandleCommand: func(_ context.Context, command Command) (CommandResult, error) {
		if command.Type != "probe_incus_application" || command.Generation != 2 {
			t.Fatalf("command = %#v", command)
		}
		return CommandResult{Status: "passing", DurationMS: 17}, nil
	}}
	if err := a.Beat(context.Background(), &saved{Credential: "shph_command"}); err != nil {
		t.Fatal(err)
	}
	if resultRequests.Load() != 1 {
		t.Fatalf("result submissions = %d", resultRequests.Load())
	}
}

func TestHeartbeatDoesNotExecuteUnknownCommandWithoutHandler(t *testing.T) {
	resultRequests := atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/servers/heartbeat" {
			_, _ = w.Write([]byte(`{"command":{"id":"2d6a4a79-9c0f-46df-83da-8ec9cde05a11","type":"probe_incus_application"}}`))
			return
		}
		resultRequests.Add(1)
	}))
	defer srv.Close()
	a := &Agent{Cfg: Config{ConsoleURL: srv.URL}}
	if err := a.Beat(context.Background(), &saved{Credential: "shph_command"}); err == nil {
		t.Fatal("command without handler was accepted")
	}
	if resultRequests.Load() != 0 {
		t.Fatal("unhandled command result was submitted")
	}
}
