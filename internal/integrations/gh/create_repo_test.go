package gh

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCreateOrgRepo(t *testing.T) {
	var perms map[string]string
	var body map[string]any
	a, _ := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/access_tokens"):
			var in struct{ Permissions map[string]string }
			_ = json.NewDecoder(r.Body).Decode(&in)
			perms = in.Permissions
			_ = json.NewEncoder(w).Encode(map[string]any{"token": "ghs_" + strings.Repeat("a", 36), "expires_at": time.Now().Add(time.Hour)})
		case r.URL.Path == "/orgs/acme/repos" && r.Method == "POST":
			if r.Header.Get("Authorization") != "token ghs_"+strings.Repeat("a", 36) {
				w.WriteHeader(401)
				return
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			w.WriteHeader(201)
			_, _ = w.Write([]byte(`{"id":77,"full_name":"acme/site","private":true,"default_branch":"main"}`))
		default:
			w.WriteHeader(404)
		}
	})
	r, err := a.CreateOrgRepo(context.Background(), 5, "acme", "Organization", "site")
	if err != nil || r.ID != 77 || r.FullName != "acme/site" {
		t.Fatalf("%+v %v", r, err)
	}
	if perms["administration"] != "write" || body["private"] != true || body["name"] != "site" {
		t.Fatalf("perms %v body %v", perms, body)
	}
	if _, err := a.CreateOrgRepo(context.Background(), 5, "operator", "User", "site"); !errors.Is(err, ErrUserAccount) {
		t.Fatalf("user account: %v", err)
	}
}
