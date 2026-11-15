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

func TestGenerateJITConfig(t *testing.T) {
	tok := "ghs_" + strings.Repeat("j", 36)
	var tokenReq struct {
		RepositoryIDs []int64           `json:"repository_ids"`
		Permissions   map[string]string `json:"permissions"`
	}
	var path, auth string
	var body map[string]any
	answer := 201
	a, _ := testApp(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/access_tokens"):
			_ = json.NewDecoder(r.Body).Decode(&tokenReq)
			_ = json.NewEncoder(w).Encode(map[string]any{"token": tok, "expires_at": time.Unix(1_800_003_600, 0)})
		case strings.HasSuffix(r.URL.Path, "/actions/runners/generate-jitconfig") && r.Method == "POST":
			path, auth = r.URL.EscapedPath(), r.Header.Get("Authorization")
			_ = json.NewDecoder(r.Body).Decode(&body)
			w.WriteHeader(answer)
			if answer == 201 {
				_, _ = w.Write([]byte(`{"runner":{"id":4242,"name":"shr-1"},"encoded_jit_config":"c2VjcmV0"}`))
			} else {
				_, _ = w.Write([]byte(`{"message":"echoes c2VjcmV0"}`))
			}
		default:
			w.WriteHeader(404)
		}
	})
	ctx := context.Background()
	cfg, id, err := a.GenerateJITConfig(ctx, 5, 77, "acme/my.app", "shr-1", []string{"shippin-2x"})
	if err != nil || cfg != "c2VjcmV0" || id != 4242 {
		t.Fatalf("got %q %d %v", cfg, id, err)
	}
	if len(tokenReq.RepositoryIDs) != 1 || tokenReq.RepositoryIDs[0] != 77 || len(tokenReq.Permissions) != 1 || tokenReq.Permissions["administration"] != "write" {
		t.Fatalf("the token is not narrowed to the repository and administration: write: %+v", tokenReq)
	}
	if path != "/repos/acme/my.app/actions/runners/generate-jitconfig" || auth != "token "+tok {
		t.Fatalf("request %s with %q", path, auth)
	}
	if body["name"] != "shr-1" || body["runner_group_id"] != float64(1) || body["work_folder"] != "_work" {
		t.Fatalf("body %v", body)
	}
	if l, _ := body["labels"].([]any); len(l) != 1 || l[0] != "shippin-2x" {
		t.Fatalf("labels %v", body["labels"])
	}

	// Path segments are escaped, so a name cannot reach another route.
	if _, _, err := a.GenerateJITConfig(ctx, 5, 78, "ac me/x?y", "shr-2", []string{"shippin-2x"}); err != nil {
		t.Fatal(err)
	}
	if path != "/repos/ac%20me/x%3Fy/actions/runners/generate-jitconfig" {
		t.Fatalf("escaped path: %s", path)
	}
	for _, bad := range []string{"acme", "/app", "acme/", "acme/app/extra"} {
		if _, _, err := a.GenerateJITConfig(ctx, 5, 77, bad, "shr-1", []string{"shippin-2x"}); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if _, _, err := a.GenerateJITConfig(ctx, 5, 77, "acme/app", "shr-1", nil); err == nil {
		t.Error("no labels accepted")
	}

	// GitHub's refusal is reported by status only; its body (which may echo the request) is withheld.
	answer = 409
	_, _, err = a.GenerateJITConfig(ctx, 5, 77, "acme/app", "shr-1", []string{"shippin-2x"})
	var se *StatusError
	if !errors.As(err, &se) || se.Code != 409 || strings.Contains(err.Error(), "c2VjcmV0") {
		t.Fatalf("refusal: %v", err)
	}
	answer = 200 // an answer without a configuration
	if _, _, err := a.GenerateJITConfig(ctx, 5, 77, "acme/app", "shr-1", []string{"shippin-2x"}); err == nil {
		t.Fatal("an empty answer was accepted")
	}
}
