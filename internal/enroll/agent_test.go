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
