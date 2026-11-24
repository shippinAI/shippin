package gh

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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testApp(t *testing.T, h http.HandlerFunc) (*App, *rsa.PrivateKey) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	p := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	a, err := New("Iv1.test", "cs", string(p), srv.URL, srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	a.now = func() time.Time { return time.Unix(1_800_000_000, 0) }
	return a, key
}

func TestJWTClaimsAndSignature(t *testing.T) {
	a, key := testApp(t, nil)
	j, err := a.JWT()
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(j, ".")
	raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var c struct {
		Iat, Exp int64
		Iss      string
	}
	_ = json.Unmarshal(raw, &c)
	if c.Iat != 1_800_000_000-60 || c.Exp != 1_800_000_000+540 || c.Iss != "Iv1.test" {
		t.Fatalf("claims %+v", c)
	}
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, sum[:], sig) != nil {
		t.Fatal("RS256 signature does not verify")
	}
	if _, err := New("x", "y", "not pem", "https://a", "https://b"); err == nil {
		t.Fatal("bad key accepted")
	}
}

func TestInstallationTokenNarrowedAndCached(t *testing.T) {
	var calls atomic.Int32
	var body map[string]any
	a, _ := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") || r.URL.Path != "/app/installations/7/access_tokens" {
			w.WriteHeader(401)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		fmt.Fprintf(w, `{"token":"ghs_abc","expires_at":"%s"}`, time.Unix(1_800_003_600, 0).UTC().Format(time.RFC3339))
	})
	ctx := context.Background()
	perms := map[string]string{"contents": "write"}
	for range 2 {
		if tok, err := a.InstallationToken(ctx, 7, []int64{42}, perms); err != nil || tok.Value != "ghs_abc" {
			t.Fatalf("%v %v", tok, err)
		}
	}
	if calls.Load() != 1 || fmt.Sprint(body["repository_ids"]) != "[42]" || fmt.Sprint(body["permissions"]) != "map[contents:write]" {
		t.Fatalf("calls %d body %v", calls.Load(), body)
	}
	a.now = func() time.Time { return time.Unix(1_800_000_000+51*60, 0) }
	_, _ = a.InstallationToken(ctx, 7, []int64{42}, perms)
	if calls.Load() != 2 {
		t.Fatal("token reused past 50 minutes")
	}
}

func TestVerifyInstallRejectsForgedID(t *testing.T) {
	a, _ := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login/oauth/access_token":
			_, _ = w.Write([]byte(`{"access_token":"ghu_user"}`))
		case "/user/installations":
			_, _ = w.Write([]byte(`{"installations":[{"id":7,"account":{"login":"operator","type":"User"},"repository_selection":"selected"}]}`))
		}
	})
	ctx := context.Background()
	if in, err := a.VerifyInstall(ctx, "code", 7); err != nil || in.Account.Login != "operator" {
		t.Fatalf("%+v %v", in, err)
	}
	if _, err := a.VerifyInstall(ctx, "code", 8); err != ErrNotYours {
		t.Fatalf("forged installation id: %v", err)
	}
}

func TestWebhookSignature(t *testing.T) {
	body := []byte(`{"action":"deleted","installation":{"id":7}}`)
	m := hmac.New(sha256.New, []byte("s3"))
	m.Write(body)
	good := "sha256=" + hex.EncodeToString(m.Sum(nil))
	if !VerifySignature("s3", body, good) {
		t.Fatal("valid signature rejected")
	}
	for _, h := range []string{"", "sha1=00", "sha256=zz", good[:len(good)-2] + "00"} {
		if VerifySignature("s3", body, h) {
			t.Fatalf("accepted %q", h)
		}
	}
	if VerifySignature("", body, good) {
		t.Fatal("empty secret must never verify")
	}
	if e, err := ParseEvent(body); err != nil || e.Action != "deleted" || e.Installation.ID != 7 {
		t.Fatalf("%+v %v", e, err)
	}
}
