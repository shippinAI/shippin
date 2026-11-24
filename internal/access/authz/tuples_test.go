package authz

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func chain() ProjectChain {
	return ProjectChain{TenantID: "t1", OrgID: "395263804486713346", ProjectID: "prj_01HF7YAT000000000000000000",
		DevspaceID: "w1", Namespace: "prj_01HF7YAT000000000000000000", OwnerSub: "395290028818825218", LinkDevspace: true}
}

func TestProjectChainTuples(t *testing.T) {
	got := chain().Tuples()
	if len(got) != 7 {
		t.Fatalf("got %d tuples", len(got))
	}
	want := map[Tuple]bool{
		{"tenant:t1", "tenant", "org:395263804486713346"}:                              true,
		{"user:395290028818825218", "owner", "project:prj_01HF7YAT000000000000000000"}: true,
		{"devspace:w1", "devspace", "rrflow_namespace:prj_01HF7YAT000000000000000000"}: true,
		{"project:prj_01HF7YAT000000000000000000", "project", "devspace:w1"}:           true,
		{"org:395263804486713346", "org", "project:prj_01HF7YAT000000000000000000"}:    true,
	}
	for _, g := range got {
		delete(want, g)
	}
	if len(want) != 0 {
		t.Errorf("missing tuples: %v", want)
	}
	for _, g := range got {
		if g.Object == "org:395263804486713346" && g.Relation != "tenant" {
			t.Errorf("a create must not grant an org role: %v", g)
		}
	}
	shared := chain()
	shared.LinkDevspace = false
	for _, g := range shared.Tuples() {
		if g.Object == "devspace:w1" || g.User == "devspace:w1" {
			t.Errorf("unlinked project got a devspace tuple: %v", g)
		}
	}
	if n := len(shared.Tuples()); n != 5 {
		t.Errorf("unlinked: %d tuples", n)
	}
	c := chain()
	c.OrgID = ""
	if c.Valid() == nil {
		t.Error("chain without org accepted")
	}
	if chain().Valid() != nil {
		t.Error("full chain refused")
	}
}

func TestWriteSendsOneIdempotentTransaction(t *testing.T) {
	var body writeBody
	var path, authz string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, authz = r.URL.Path, r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte("{}"))
	}))
	defer srv.Close()
	f := NewFGA(srv.URL, "s1", "m1", "tok", time.Second)
	if err := f.Write(context.Background(), chain().Tuples()); err != nil {
		t.Fatal(err)
	}
	if path != "/stores/s1/write" || authz != "Bearer tok" || body.AuthorizationModelID != "m1" ||
		body.Writes.OnDuplicate != "ignore" || len(body.Writes.TupleKeys) != 7 {
		t.Fatalf("path=%s auth=%q body=%+v", path, authz, body)
	}
	if f.Write(context.Background(), nil) == nil {
		t.Error("empty write accepted")
	}
}

func TestWriteFailsClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusBadRequest)
	}))
	defer srv.Close()
	if err := NewFGA(srv.URL, "s", "m", "", time.Second).Write(context.Background(), chain().Tuples()); err == nil {
		t.Fatal("400 treated as success")
	}
}
