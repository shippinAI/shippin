package stateapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shippin.cloud/console/internal/rrflow/state"
)

const (
	org = "019a0000-0000-7000-8000-00000000f001"
	sbA = "019a0000-0000-7000-8000-0000000000a1"
	sbB = "019a0000-0000-7000-8000-0000000000b2"
	wsA = "019a0000-0000-7000-8000-0000000000c3"
	wsB = "019a0000-0000-7000-8000-0000000000d4"
)

var tokA, tokB = strings.Repeat("a", 43), strings.Repeat("b", 43)

func call(h http.Handler, method, path, tok, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if tok != "" {
		r.Header.Set("Authorization", "Bearer "+tok)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestBindingScopesEveryCall(t *testing.T) {
	dir := t.TempDir()
	st, _ := state.Open(dir, org)
	b := &Bindings{Path: filepath.Join(dir, "bindings.json")}
	if b.Set(sbA, wsA, "", tokA) != nil || b.Set(sbB, wsB, "", tokB) != nil {
		t.Fatal("bind")
	}
	_ = st
	h := (&Server{Stores: &state.Namespaces{Root: dir, Org: org}, Bindings: b}).Routes()
	if w := call(h, "GET", "/v1/memory", "", ""); w.Code != 401 {
		t.Fatalf("no token: %d", w.Code)
	}
	if w := call(h, "GET", "/v1/memory", strings.Repeat("c", 43), ""); w.Code != 401 {
		t.Fatalf("unknown token: %d", w.Code)
	}
	if w := call(h, "PUT", "/v1/memory/plan/next", tokA, `{"value":"add auth"}`); w.Code != 200 {
		t.Fatalf("put: %d %s", w.Code, w.Body)
	}
	if w := call(h, "GET", "/v1/memory/plan/next", tokB, ""); w.Code != 404 {
		t.Fatal("workspace B read workspace A's memory")
	}
	if w := call(h, "POST", "/v1/decisions", tokA, `{"action":"file.write","effect":"allow","resource":"src/App.tsx"}`); w.Code != 201 {
		t.Fatalf("decision: %d %s", w.Code, w.Body)
	}
	if w := call(h, "GET", "/v1/decisions", tokB, ""); !strings.Contains(w.Body.String(), `"items":[]`) {
		t.Fatalf("decisions leaked: %s", w.Body)
	}
	if b.Remove(sbA) != nil {
		t.Fatal("unbind")
	}
	if w := call(h, "GET", "/v1/memory", tokA, ""); w.Code != 401 {
		t.Fatal("revoked token still works")
	}
	if strings.Contains(readFile(t, b.Path), tokB) {
		t.Fatal("token stored in clear")
	}
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// D36: two projects share one instance; each binding sees only its own namespace.
func TestNamespacesIsolateProjects(t *testing.T) {
	dir := t.TempDir()
	nsA, nsB := "019a0000-0000-7000-8000-0000000000e5", "019a0000-0000-7000-8000-0000000000f6"
	b := &Bindings{Path: filepath.Join(dir, "bindings.json")}
	if b.Set(sbA, wsA, nsA, tokA) != nil || b.Set(sbB, wsA, nsB, tokB) != nil {
		t.Fatal("bind")
	}
	if b.Set(sbA, wsA, "../x", tokA) == nil {
		t.Fatal("bad namespace accepted")
	}
	h := (&Server{Stores: &state.Namespaces{Root: dir, Org: org}, Bindings: b}).Routes()
	if w := call(h, "PUT", "/v1/memory/k", tokA, `{"value":"a"}`); w.Code != 200 {
		t.Fatalf("put: %d", w.Code)
	}
	if w := call(h, "GET", "/v1/memory/k", tokB, ""); w.Code != 404 {
		t.Fatal("namespace B read namespace A (same workspace id)")
	}
	if _, err := os.Stat(filepath.Join(dir, "ns", nsA, "memory.jsonl")); err != nil {
		t.Fatalf("namespace dir: %v", err)
	}
}
