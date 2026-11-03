package approval

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPBoardPostsAndReads(t *testing.T) {
	var got MethodBody
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got = MethodBody{r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization"), string(b)}
		_ = json.NewEncoder(w).Encode(Decision{Key: "gold.ticket:T1", Status: "needs_approval"})
	}))
	defer srv.Close()
	h := HTTPBoard{BaseURL: srv.URL, Token: "sekret", OrgID: "ops"}
	d, err := h.Post(context.Background(), Request{Key: "gold.ticket:T1", Title: "Approve T1", By: "agent"})
	if err != nil || d.Status != "needs_approval" {
		t.Fatalf("%+v %v", d, err)
	}
	if got.Method != http.MethodPost || got.Path != "/v1/hub/approvals" || got.Auth != "Bearer sekret" {
		t.Fatalf("post: %+v", got)
	}
	if _, err := h.Status(context.Background(), "ops", "gold.ticket:T1"); err != nil {
		t.Fatal(err)
	}
	if got.Method != http.MethodGet || got.Path != "/v1/hub/approvals/gold.ticket:T1" {
		t.Fatalf("get: %+v", got)
	}
}

type MethodBody struct {
	Method, Path, Query, Auth, Body string
}
