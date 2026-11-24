package runfleet

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRedactPrivateKeyAndQuotedPassword(t *testing.T) {
	in := "-----BEGIN PRIVATE KEY-----\nprivate-material\n-----END PRIVATE KEY-----\npassword=\"multiple secret words\"\nsecret='other private words'"
	out, _ := Redact(in)
	for _, secret := range []string{"private-material", "multiple", "secret words", "other", "private words"} {
		if strings.Contains(out, secret) {
			t.Fatalf("credential fragment survived: %q", secret)
		}
	}
}

func TestRetentionPurgesExistingBodyFromDisk(t *testing.T) {
	for _, expire := range []bool{false, true} {
		t.Run(map[bool]string{false: "zero", true: "expired"}[expire], func(t *testing.T) {
			svc := testService(t, nil)
			_, err := svc.Connect(t.Context(), ConnectInput{Account: "octo", AccountKind: "user", Repos: []string{"octo/repo"}, Size: "linux-x64-2", Facts: []RepoFact{{FullName: "octo/repo", License: "MIT"}}})
			if err != nil {
				t.Fatal(err)
			}
			job, err := svc.IngestJob(IngestInput{Repo: "octo/repo", Conclusion: "failure", Log: "retained-log-body", StartedAt: svc.now().Add(-time.Minute), FinishedAt: svc.now()})
			if err != nil {
				t.Fatal(err)
			}
			if expire {
				now := svc.now().AddDate(0, 0, 14)
				svc.now = func() time.Time { return now }
			} else if err := svc.SetRetention("octo/repo", 0); err != nil {
				t.Fatal(err)
			}
			logs, err := svc.JobLogs(job.ID)
			if err != nil || !logs.Expired || logs.Stored || logs.Log != "" {
				t.Fatalf("logs=%+v err=%v", logs, err)
			}
			body, err := os.ReadFile(filepath.Join(svc.StateDir(), "state.json"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(body), "retained-log-body") {
				t.Fatal("expired log remains on disk")
			}
		})
	}
}

func TestCredentialCallsDoNotFollowRedirects(t *testing.T) {
	for _, provider := range []string{"github", "proxmox"} {
		t.Run(provider, func(t *testing.T) {
			followed := false
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/redirect-target" {
					followed = true
					w.WriteHeader(http.StatusOK)
					return
				}
				http.Redirect(w, r, "/redirect-target", http.StatusTemporaryRedirect)
			}))
			defer srv.Close()
			if provider == "github" {
				g := &HTTPGitHub{BaseURL: srv.URL, Token: "test-token", Client: srv.Client()}
				if err := g.get(context.Background(), "/original", &struct{}{}); err == nil {
					t.Fatal("redirect accepted")
				}
			} else {
				p := Proxmox{Token: "test-token", Client: srv.Client()}
				_, status, err := p.call(context.Background(), http.MethodGet, srv.URL+"/original", nil)
				if err != nil || status != http.StatusTemporaryRedirect {
					t.Fatalf("status=%d err=%v", status, err)
				}
			}
			if followed {
				t.Fatal("credential-bearing request followed redirect")
			}
		})
	}
}

func TestExplanationDoesNotInventSuccessOrExposeSecrets(t *testing.T) {
	got := ExplainLog(Job{}, "still building", true, false)
	if got.Signals[0] != "pending_conclusion" {
		t.Fatal(got)
	}
	got = ExplainLog(Job{Conclusion: "failure"}, "##[error] password=\"secret words\"", true, false)
	if strings.Contains(got.Summary, "secret words") {
		t.Fatal("explanation exposed credential")
	}
}

func TestPriceOverflowIsRejected(t *testing.T) {
	if _, err := parseUSDPerMinute("9223372036854775808"); err == nil {
		t.Fatal("overflow accepted")
	}
}

func TestCriticalHostLoadIsCritical(t *testing.T) {
	if Compose(HealthOK, HealthOK, HealthCritical) != HealthCritical {
		t.Fatal("critical host load reported healthy")
	}
}
