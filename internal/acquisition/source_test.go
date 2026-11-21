package acquisition

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func tlsSource(t *testing.T, h http.Handler) (origin, ip string, port int, ca string) {
	t.Helper()
	s := httptest.NewTLSServer(h)
	t.Cleanup(s.Close)
	u, err := url.Parse(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, p, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err = strconv.Atoi(p)
	if err != nil {
		t.Fatal(err)
	}
	cert := s.Certificate()
	return "https://127.0.0.1", "127.0.0.1", port, "-----BEGIN CERTIFICATE-----\n" + base64.StdEncoding.EncodeToString(cert.Raw) + "\n-----END CERTIFICATE-----\n"
}

func TestHTTPFetcherPinsIPAndMintsReadOnlyShortToken(t *testing.T) {
	seed := make([]byte, ed25519.SeedSize)
	if _, err := rand.Read(seed); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	type seenRequest struct{ path, query, authorization string }
	seen := make(chan seenRequest, 1)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- seenRequest{r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization")}
		if r.Method != http.MethodGet {
			t.Errorf("method %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"version": 1, "retention_days": 30, "next_cursor": 7, "items": []Lead{{ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Email: "person@example.test", Status: "new", ConsentVersion: "early-access-v1", CreatedAt: now, ConsentAt: now, UpdatedAt: now, ExpiresAt: now.Add(30 * 24 * time.Hour), Revision: 7}}})
	})
	origin, ip, port, ca := tlsSource(t, h)
	source := SourceConfig{ID: "bbbbbbbb-bbbb-7bbb-8bbb-bbbbbbbbbbbb", Origin: origin, DialIP: ip, Port: port, CAPEM: ca, SourceTenant: "tenant", SourceProject: "project", Issuer: "https://issuer.example.test", Audience: "marketing-admin"}
	if _, _, _, err := validateSource(source); err != nil {
		t.Fatalf("source config validation: %v", err)
	}
	page, err := (HTTPFetcher{}).Fetch(context.Background(), source, seed, 6)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	got := <-seen
	if got.path != "/v1/projects/project/marketing/leads" || got.query != "after=6&limit=100" || len(page.Items) != 1 || page.Cursor != 7 || page.RetentionDays != 30 || page.Items[0].EmailVerified {
		t.Fatalf("request/page mismatch path=%q query=%q item_count=%d cursor=%d retention=%d", got.path, got.query, len(page.Items), page.Cursor, page.RetentionDays)
	}
	if !strings.HasPrefix(got.authorization, "Bearer ") {
		t.Fatal("missing delegated bearer")
	}
	parts := strings.Split(strings.TrimPrefix(got.authorization, "Bearer "), ".")
	if len(parts) != 3 {
		t.Fatal("invalid bearer shape")
	}
	claimsRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal("invalid bearer payload")
	}
	var claims sourceClaims
	if json.Unmarshal(claimsRaw, &claims) != nil || claims.Issuer != source.Issuer || claims.Audience != source.Audience || claims.Subject != "acquisition:"+source.ID || claims.Tenant != "tenant" || len(claims.Projects) != 1 || claims.Projects[0] != "project" || len(claims.Scopes) != 1 || claims.Scopes[0] != "marketing:read" || claims.Expiry-claims.NotBefore != 120 {
		t.Fatalf("incorrectly scoped token claims: %+v", claims)
	}
}

func TestDecodePageRejectsMalformedPagesAndAcceptsBareTombstone(t *testing.T) {
	now := time.Now().UTC()
	valid := func(l Lead) []byte {
		b, _ := json.Marshal(envelope{Version: 1, Items: []Lead{l}, NextCursor: l.Revision, RetentionDays: 30})
		return b
	}
	for _, tc := range []struct {
		name string
		lead Lead
	}{
		{"bad id", Lead{ID: "not-an-id", Revision: 1}},
		{"bad status", Lead{ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Revision: 1, Email: "person@example.test", Status: "unknown", ConsentVersion: "early-access-v1", CreatedAt: now, ConsentAt: now, UpdatedAt: now, ExpiresAt: now.Add(time.Hour)}},
		{"tombstone carries PII", Lead{ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Revision: 1, Deleted: true, Email: "person@example.test"}},
	} {
		if _, err := decodePage(valid(tc.lead), 0); err == nil {
			t.Errorf("%s accepted", tc.name)
		}
	}
	tombstone := Lead{ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Revision: 8, Deleted: true}
	page, err := decodePage(valid(tombstone), 7)
	if err != nil || len(page.Items) != 1 || !page.Items[0].Deleted || page.Items[0].Email != "" || !page.Items[0].CreatedAt.IsZero() {
		t.Fatalf("bare tombstone rejected or expanded: %+v %v", page, err)
	}
	if _, err := decodePage([]byte(`{"version":1,"items":[],"next_cursor":1,"retention_days":30}`), 0); err == nil {
		t.Fatal("cursor advanced without items")
	}
	for _, raw := range []string{
		`{"version":1,"version":1,"items":[],"next_cursor":0,"retention_days":30}`,
		`{"version":1,"items":[{"id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","revision":1,"deleted":true}],"next_cursor":1,"retention_days":30}`,
		`{"version":1,"items":[{"ID":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","revision":1,"deleted":true}],"next_cursor":1,"retention_days":30}`,
		`{"version":1,"items":null,"next_cursor":0,"retention_days":30}`,
	} {
		if _, err := decodePage([]byte(raw), 0); err == nil {
			t.Errorf("ambiguous or noncanonical source JSON accepted")
		}
	}
	if _, err := decodePage([]byte(`{"version":1,"items":[],"next_cursor":0,"retention_days":30,"identity":"other"}`), 0); err == nil {
		t.Fatal("unknown envelope identity accepted")
	}
}

func TestValidateSourceRefusesUnsafeAndUnpinnedTargets(t *testing.T) {
	base := SourceConfig{ID: "bbbbbbbb-bbbb-7bbb-8bbb-bbbbbbbbbbbb", Origin: "https://example.test", DialIP: "127.0.0.1", Port: 443, SourceTenant: "tenant", SourceProject: "project", Issuer: "issuer", Audience: "audience", CAPEM: "not pem"}
	for _, mutate := range []func(*SourceConfig){
		func(s *SourceConfig) { s.Origin = "https://user@example.test" },
		func(s *SourceConfig) { s.Origin = "https://example.test/path" },
		func(s *SourceConfig) { s.Origin = "https://example.test:8443" },
		func(s *SourceConfig) { s.Origin = "https://metadata.google.internal" },
		func(s *SourceConfig) { s.DialIP = "169.254.169.254" },
		func(s *SourceConfig) { s.DialIP = net.IPv4(100, 100, 100, 200).String() },
		func(s *SourceConfig) { s.DialIP = "fd00:ec2::254" },
		func(s *SourceConfig) { s.DialIP = "0.0.0.0" },
		func(s *SourceConfig) { s.DialIP = "224.0.0.1" },
		func(s *SourceConfig) { s.DialIP = "not-an-ip" },
	} {
		s := base
		mutate(&s)
		if _, _, _, err := validateSource(s); err == nil {
			t.Errorf("unsafe source accepted")
		}
	}
}

func TestValidateSourceAcceptsOnlyMatchingExplicitURLPort(t *testing.T) {
	origin, ip, port, ca := tlsSource(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	source := SourceConfig{ID: "bbbbbbbb-bbbb-7bbb-8bbb-bbbbbbbbbbbb", Origin: origin + ":" + strconv.Itoa(port), DialIP: ip, Port: port,
		SourceTenant: "tenant", SourceProject: "project", Issuer: "issuer", Audience: "audience", CAPEM: ca}
	if _, _, _, err := validateSource(source); err != nil {
		t.Fatalf("matching explicit URL port rejected: %v", err)
	}
	source.Port++
	if _, _, _, err := validateSource(source); err == nil {
		t.Fatal("mismatched explicit URL and dial ports accepted")
	}
}

func TestHTTPFetcherRefusesRedirectsAndSanitizesNetworkErrors(t *testing.T) {
	origin, ip, port, ca := tlsSource(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://other.example.test/steal", http.StatusFound)
	}))
	seed := make([]byte, ed25519.SeedSize)
	source := SourceConfig{ID: "bbbbbbbb-bbbb-7bbb-8bbb-bbbbbbbbbbbb", Origin: origin, DialIP: ip, Port: port, CAPEM: ca, SourceTenant: "tenant", SourceProject: "project", Issuer: "issuer", Audience: "audience"}
	_, err := (HTTPFetcher{}).Fetch(context.Background(), source, seed, 0)
	if err == nil || err.Error() != ErrUnavailable.Error() || strings.Contains(err.Error(), "example.test") {
		t.Fatalf("redirect/error not sanitized: %v", err)
	}
}
