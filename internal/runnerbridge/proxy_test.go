package runnerbridge

import (
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProxyPreservesOnlySessionAndSafeResponse(t *testing.T) {
	calls := 0
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/runner/projects" || len(r.Cookies()) != 1 || r.Header.Get("Authorization") != "" || r.Header.Get("X-Forwarded-User") != "" {
			t.Error("unexpected request forwarding")
		}
		cookie, e := r.Cookie("shippin_session")
		if e != nil || cookie.Value != "synthetic-session-value" {
			t.Error("missing exact session")
		}
		w.Header().Set("Set-Cookie", "secret=do-not-forward")
		w.Header().Set("X-Private", "do-not-forward")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"projects":[]}`))
	}))
	defer upstream.Close()
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if e := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: upstream.Certificate().Raw}), 0600); e != nil {
		t.Fatal(e)
	}
	p, e := New(upstream.URL, ca)
	if e != nil {
		t.Fatal(e)
	}
	mux := http.NewServeMux()
	p.Register(mux)
	request := func(authenticated bool) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/v1/runner/projects", nil)
		if authenticated {
			r.AddCookie(&http.Cookie{Name: "shippin_session", Value: "synthetic-session-value"})
			r.AddCookie(&http.Cookie{Name: "other", Value: "must-not-forward"})
		}
		r.Header.Set("Authorization", "Bearer must-not-forward")
		r.Header.Set("X-Forwarded-User", "owner")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, r)
		return w
	}
	if w := request(false); w.Code != 401 || calls != 0 {
		t.Fatal("anonymous request reached gateway")
	}
	w := request(true)
	if w.Code != 200 || w.Body.String() != `{"projects":[]}` || w.Header().Get("Set-Cookie") != "" || w.Header().Get("X-Private") != "" {
		t.Fatalf("unsafe response: %v %s", w.Code, w.Body.String())
	}
	r := httptest.NewRequest("GET", "/v1/runner/projects", nil)
	r.Header.Set("Cookie", "shippin_session=synthetic-session-value; shippin_session=synthetic-session-value")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != 401 || calls != 1 {
		t.Fatal("duplicate session accepted")
	}
}
func TestProxyUnavailableAndInvalidConfiguration(t *testing.T) {
	p, e := New("", "")
	if e != nil {
		t.Fatal(e)
	}
	w := httptest.NewRecorder()
	p.ServeHTTP(w, httptest.NewRequest("GET", "/v1/runner/projects", nil))
	if w.Code != 503 || !strings.Contains(w.Body.String(), "not configured") {
		t.Fatal("missing config disguised as available")
	}
	for _, origin := range []string{"http://localhost", "https://example.test/path", "https://u:p@example.test", "https://example.test?", "https://example.test#x"} {
		if _, e := New(origin, "missing"); e == nil {
			t.Fatal("invalid config accepted")
		}
	}
}
func TestProxyDoesNotFollowRedirectOrExposeUpstreamError(t *testing.T) {
	hit := false
	sink := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
	defer sink.Close()
	redirect := true
	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if redirect {
			http.Redirect(w, r, sink.URL, 302)
		} else {
			http.Error(w, "secret upstream error", 500)
		}
	}))
	defer source.Close()
	ca := filepath.Join(t.TempDir(), "ca.pem")
	_ = os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: source.Certificate().Raw}), 0600)
	p, e := New(source.URL, ca)
	if e != nil {
		t.Fatal(e)
	}
	for _, value := range []bool{true, false} {
		redirect = value
		r := httptest.NewRequest("GET", "/v1/runner/projects", nil)
		r.AddCookie(&http.Cookie{Name: "shippin_session", Value: "synthetic-session-value"})
		w := httptest.NewRecorder()
		p.ServeHTTP(w, r)
		if w.Code != 502 || hit || strings.Contains(w.Body.String(), "secret") {
			t.Fatal("unsafe upstream response")
		}
	}
}
