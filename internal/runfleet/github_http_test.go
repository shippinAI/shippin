package runfleet

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPGitHubOpenPullAndRepo(t *testing.T) {
	var pulls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("auth %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/repos/octo/hello" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"full_name":"octo/hello","private":false,"default_branch":"main","license":{"spdx_id":"MIT"}}`))
		case strings.Contains(r.URL.Path, "/contents/.github/workflows/ci.yml"):
			_, _ = w.Write([]byte(`{"encoding":"base64","content":"` + base64.StdEncoding.EncodeToString([]byte("runs-on: ubuntu-latest\n")) + `"}`))
		case strings.HasSuffix(r.URL.Path, "/contents/.github/workflows"):
			_, _ = w.Write([]byte(`[{"name":"ci.yml","path":".github/workflows/ci.yml","type":"file"}]`))
		case strings.Contains(r.URL.Path, "/contents/.lovable/project.json"):
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"Not Found"}`))
		case strings.HasSuffix(r.URL.Path, "/git/ref/heads/main"):
			_, _ = w.Write([]byte(`{"object":{"sha":"abc"}}`))
		case strings.HasSuffix(r.URL.Path, "/git/blobs"):
			_, _ = w.Write([]byte(`{"sha":"blob"}`))
		case strings.HasSuffix(r.URL.Path, "/git/trees"):
			_, _ = w.Write([]byte(`{"sha":"tree"}`))
		case strings.HasSuffix(r.URL.Path, "/git/commits"):
			_, _ = w.Write([]byte(`{"sha":"commit"}`))
		case strings.HasSuffix(r.URL.Path, "/git/refs"):
			_, _ = w.Write([]byte(`{"ref":"refs/heads/shippin"}`))
		case strings.HasSuffix(r.URL.Path, "/pulls"):
			pulls++
			_, _ = w.Write([]byte(`{"html_url":"https://github.com/octo/hello/pull/1"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	gh := &HTTPGitHub{BaseURL: srv.URL, Token: "installation-token", Client: srv.Client()}
	fact, err := gh.Repo(context.Background(), "octo/hello")
	if err != nil {
		t.Fatal(err)
	}
	if fact.License != "MIT" || fact.Lovable || len(fact.Workflows) != 1 {
		t.Fatalf("%+v", fact)
	}
	url, err := gh.OpenPull(context.Background(), "octo/hello", "title", "body", "shippin-run/abc", []FileChange{{
		Path: ".github/workflows/ci.yml", Content: "runs-on: shippin-linux-x64-2\n",
	}})
	if err != nil || url != "https://github.com/octo/hello/pull/1" || pulls != 1 {
		t.Fatalf("url=%s pulls=%d err=%v", url, pulls, err)
	}
}

func TestAppJWT(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	token, err := AppJWT("123", key, time.Unix(1_700_000_000, 0))
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatal(token)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatal(err)
	}
	if claims["iss"] != "123" {
		t.Fatal(claims)
	}
	signed := parts[0] + "." + parts[1]
	sum := sha256.Sum256([]byte(signed))
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, sum[:], sig); err != nil {
		t.Fatal(err)
	}
}
