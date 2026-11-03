package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/version"
)

func compareServer(t *testing.T, status int, body string, saw *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if saw != nil {
			*saw = r.URL.Path
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func updatesCfg(url string) config.Updates {
	return config.Updates{Enabled: true, Repository: "owner/repo", Branch: "main", APIURL: url}
}

func TestUpdateCheckReportsANewPush(t *testing.T) {
	var saw string
	srv := compareServer(t, http.StatusOK, `{"status":"ahead","ahead_by":2,"html_url":"https://example.invalid/compare",
		"commits":[{"sha":"aaa","commit":{"message":"First","committer":{"date":"2026-11-01T00:00:00Z"}}},
		           {"sha":"bbb","commit":{"message":"Latest thing\n\nLong body","committer":{"date":"2026-11-02T00:00:00Z"}}}]}`, &saw)
	got := (&updateChecker{}).read(context.Background(), updatesCfg(srv.URL), version.Info{Commit: "abc123"})
	if !got.Available || got.Behind != 2 || got.Latest != "bbb" || got.Title != "Latest thing" || got.Date != "2026-11-02T00:00:00Z" || got.CompareURL == "" || got.Error != "" {
		t.Fatalf("%+v", got)
	}
	if saw != "/repos/owner/repo/compare/abc123...main" {
		t.Fatalf("path %q", saw)
	}
}

func TestUpdateCheckIsNotAvailableWhenCurrentOrAhead(t *testing.T) {
	for _, body := range []string{`{"status":"identical","ahead_by":0}`, `{"status":"behind","ahead_by":0}`} {
		srv := compareServer(t, http.StatusOK, body, nil)
		if got := (&updateChecker{}).read(context.Background(), updatesCfg(srv.URL), version.Info{Commit: "abc123"}); got.Available || got.Error != "" {
			t.Errorf("%s: %+v", body, got)
		}
	}
}

func TestUpdateCheckNeverClaimsCurrentWhenItCouldNotTell(t *testing.T) {
	notFound := compareServer(t, http.StatusNotFound, `{}`, nil)
	limited := compareServer(t, http.StatusForbidden, `{"message":"rate limit"}`, nil)
	garbled := compareServer(t, http.StatusOK, `not json`, nil)
	gone := compareServer(t, http.StatusOK, `{}`, nil)
	gone.Close()
	for name, u := range map[string]string{"not on the branch": notFound.URL, "rate limited": limited.URL, "garbled": garbled.URL, "unreachable": gone.URL} {
		got := (&updateChecker{}).read(context.Background(), updatesCfg(u), version.Info{Commit: "abc123"})
		if got.Available || got.Error == "" {
			t.Errorf("%s: %+v", name, got)
		}
	}
	if got := (&updateChecker{}).read(context.Background(), updatesCfg(notFound.URL), version.Info{}); got.Available || got.Error == "" {
		t.Errorf("no commit stamp: %+v", got)
	}
}

func TestUpdateCommandComesFromHowItWasDeployed(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if got := updateCommand(config.Updates{}, env(map[string]string{"SHIPPIN_DEPLOY_METHOD": "compose"})); !strings.Contains(got, "docker compose up") {
		t.Errorf("compose: %q", got)
	}
	got := updateCommand(config.Updates{}, env(map[string]string{"SHIPPIN_DEPLOY_METHOD": "installer", "SHIPPIN_SOURCE_DIR": "/opt/x", "SHIPPIN_SITE_FILE": "/etc/x/site.env"}))
	if !strings.Contains(got, "cd /opt/x") || !strings.Contains(got, "--site /etc/x/site.env") {
		t.Errorf("installer: %q", got)
	}
	if got := updateCommand(config.Updates{}, env(nil)); got != "" {
		t.Errorf("unknown deployment must not guess: %q", got)
	}
	if got := updateCommand(config.Updates{Command: "site-update"}, env(map[string]string{"SHIPPIN_DEPLOY_METHOD": "compose"})); got != "site-update" {
		t.Errorf("configured command: %q", got)
	}
}

func TestVersionNeedsAnOperator(t *testing.T) {
	a := infraAPI(t, "", &memHV{})
	rec := httptest.NewRecorder()
	a.versionInfo(rec, httptest.NewRequest(http.MethodGet, "/v1/version", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("no operator role: %d", rec.Code)
	}
	a = infraAPI(t, "op_support", &memHV{})
	a.resolveCaller = func(http.ResponseWriter, *http.Request) (*caller, bool) {
		return &caller{id: &auth.Identity{Sub: "u"}, op: "op_support"}, true
	}
	rec = httptest.NewRecorder()
	a.versionInfo(rec, httptest.NewRequest(http.MethodGet, "/v1/version", nil))
	var out map[string]any
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &out) != nil {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if _, has := out["update"]; has {
		t.Fatalf("updates disabled must not report one: %v", out)
	}
}

func TestUpdatesConfigFailsClosed(t *testing.T) {
	ok := config.Updates{Enabled: true, Repository: "o/r", Branch: "main", APIURL: "https://api.example.invalid"}
	if err := ok.Check(); err != nil {
		t.Fatal(err)
	}
	for name, u := range map[string]config.Updates{
		"bad repo":   {Enabled: true, Repository: "not a repo", Branch: "main", APIURL: "https://x"},
		"no branch":  {Enabled: true, Repository: "o/r", APIURL: "https://x"},
		"bad scheme": {Enabled: true, Repository: "o/r", Branch: "main", APIURL: "ftp://x"},
	} {
		if u.Check() == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if (config.Updates{}).Check() != nil {
		t.Error("disabled needs nothing")
	}
}
