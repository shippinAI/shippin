package vaultix

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEnsureFolderCreatesEachSegment(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/universal-auth/login" {
			json.NewEncoder(w).Encode(map[string]any{"accessToken": "t", "expiresIn": 3600})
			return
		}
		var in map[string]string
		json.NewDecoder(r.Body).Decode(&in)
		got = append(got, in["path"]+"|"+in["name"])
		if in["name"] == "workspaces" {
			w.WriteHeader(400)
			w.Write([]byte(`{"message":"Folder with name 'workspaces' already exists"}`))
		}
	}))
	defer srv.Close()
	c := New(srv.URL, "id", "sec", 5*time.Second)
	if err := c.EnsureFolder(context.Background(), "p", "prod", "/workspaces/w1/connect"); err != nil {
		t.Fatal(err)
	}
	want := []string{"/|workspaces", "/workspaces|w1", "/workspaces/w1|connect"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("%v", got)
	}
}
