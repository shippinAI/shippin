package runfleet

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testService(t *testing.T, gh GitHub) *Service {
	t.Helper()
	svc, err := Open(t.TempDir(), Options{
		Now:         func() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) },
		GitHub:      gh,
		FreeMinutes: 1,
		Provisioner: Proxmox{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestConnectPersonalAccountDowngradesOrgRegistration(t *testing.T) {
	svc := testService(t, nil)
	res, err := svc.Connect(context.Background(), ConnectInput{
		Account: "octocat", AccountKind: "user", Registration: "organization",
		Repos: []string{"octocat/hello"}, Size: "linux-x64-2",
		Facts: []RepoFact{{FullName: "octocat/hello", License: "MIT", Lovable: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Registration != "repository" || res.RegistrationNote == "" {
		t.Fatalf("%+v", res)
	}
	if res.RunsOn != "shippin-linux-x64-2" || res.GitHub != "not_configured" {
		t.Fatalf("%+v", res)
	}
	if len(res.Repos) != 1 || res.Repos[0].Approval != "auto" || res.Repos[0].Lovable != true {
		t.Fatalf("%+v", res.Repos)
	}
	if res.Repos[0].Provision.Implemented {
		t.Fatal("provisioner reported a guest it did not create")
	}
	if res.Repos[0].Runner == nil || res.Repos[0].Runner.Status != "desired" {
		t.Fatalf("runner = %+v", res.Repos[0].Runner)
	}
	if res.Repos[0].Workflow == nil || len(res.Repos[0].Workflow.Changes) != 1 {
		t.Fatal("expected a lovable workflow proposal")
	}
	if res.Repos[0].Workflow.PullURL != "" {
		t.Fatal("dry-run opened a pull request")
	}
	runners, err := svc.ListRunners()
	if err != nil || len(runners) != 1 {
		t.Fatalf("runners %+v %v", runners, err)
	}
	raw, err := os.ReadFile(filepath.Join(svc.StateDir(), "ci_runners.json"))
	if err != nil || !strings.Contains(string(raw), `"status": "desired"`) {
		t.Fatalf("fleet file: %s %v", raw, err)
	}
}

func TestOrganizationSharesOneRunner(t *testing.T) {
	svc := testService(t, nil)
	res, err := svc.Connect(context.Background(), ConnectInput{
		Account: "acme", AccountKind: "organization",
		Repos: []string{"acme/one", "acme/two"}, Size: "linux-x64-2",
		Facts: []RepoFact{
			{FullName: "acme/one", License: "MIT"},
			{FullName: "acme/two", License: "Apache-2.0"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Registration != "organization" || len(res.Repos) != 2 {
		t.Fatalf("%+v", res)
	}
	if res.Repos[0].Runner == nil || res.Repos[1].Runner == nil || res.Repos[0].Runner.ID != res.Repos[1].Runner.ID {
		t.Fatalf("repos = %+v %+v", res.Repos[0].Runner, res.Repos[1].Runner)
	}
	runners, err := svc.ListRunners()
	if err != nil || len(runners) != 1 || len(runners[0].Repos) != 2 {
		t.Fatalf("%+v %v", runners, err)
	}
}

func TestPrivateRepoDoesNotProvision(t *testing.T) {
	svc := testService(t, nil)
	res, err := svc.Connect(context.Background(), ConnectInput{
		Account: "acme", AccountKind: "organization",
		Repos: []string{"acme/secret"}, Size: "linux-arm-2",
		Facts: []RepoFact{{FullName: "acme/secret", Private: true, License: "MIT"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Registration != "organization" || res.Repos[0].Approval != "pending" || res.Repos[0].Runner != nil {
		t.Fatalf("%+v", res.Repos[0])
	}
	if !res.Repos[0].Provision.Skipped {
		t.Fatal(res.Repos[0].Provision)
	}
	runners, _ := svc.ListRunners()
	if len(runners) != 0 {
		t.Fatalf("pending repo created a runner: %+v", runners)
	}
	approved, err := svc.Approve(context.Background(), ApproveInput{
		ConnectionID: res.ConnectionID, Repo: "acme/secret", Reason: "customer confirmed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if approved.Approval != "approved" || approved.Provision.Implemented {
		t.Fatalf("%+v", approved)
	}
	runners, _ = svc.ListRunners()
	if len(runners) != 1 || runners[0].AccountKind != "organization" {
		t.Fatalf("%+v", runners)
	}
}

func TestLogsAreRedactedAndRetentionDiscardsThem(t *testing.T) {
	svc := testService(t, nil)
	res, err := svc.Connect(context.Background(), ConnectInput{
		Account: "octocat", AccountKind: "user", Repos: []string{"octocat/hello"},
		Size: "linux-x64-2", LogRetentionDays: intPtr(0),
		Facts: []RepoFact{{FullName: "octocat/hello", License: "Apache-2.0",
			Workflows: []WorkflowFile{{Path: ".github/workflows/ci.yml", Content: "jobs:\n  t:\n    runs-on: ubuntu-latest\n"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	secret := "ghp_" + strings.Repeat("b", 36)
	summary, err := svc.IngestJob(IngestInput{
		Repo: "octocat/hello", Name: "test", Conclusion: "failure",
		RunnerName: res.Repos[0].Runner.Name,
		Log:        "password=" + secret + "\n##[error]Process completed with exit code 1\n",
		StartedAt:  time.Date(2026, 11, 13, 11, 0, 0, 0, time.UTC),
		FinishedAt: time.Date(2026, 11, 13, 11, 1, 30, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.LogStored {
		t.Fatal("retention 0 stored a log")
	}
	body, _ := os.ReadFile(filepath.Join(svc.StateDir(), "state.json"))
	if strings.Contains(string(body), secret) || strings.Contains(string(body), "ghp_") {
		t.Fatalf("secret persisted:\n%s", body)
	}
	logs, err := svc.JobLogs(summary.ID)
	if err != nil || logs.Log != "" || !logs.Expired {
		t.Fatalf("%+v %v", logs, err)
	}
	explained, err := svc.Explain(summary.ID)
	if err != nil || explained.Signals[0] != "expired" {
		t.Fatalf("%+v %v", explained, err)
	}
}

func TestRedactedLogExplainsFailure(t *testing.T) {
	svc := testService(t, nil)
	_, err := svc.Connect(context.Background(), ConnectInput{
		Account: "octocat", AccountKind: "user", Repos: []string{"octocat/hello"},
		Size:  "linux-x64-2",
		Facts: []RepoFact{{FullName: "octocat/hello", License: "MIT"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	summary, err := svc.IngestJob(IngestInput{
		Repo: "octocat/hello", Name: "test", Conclusion: "failure",
		Log:        "Waiting for a runner with label shippin-linux-x64-2\n",
		StartedAt:  time.Date(2026, 11, 13, 11, 0, 0, 0, time.UTC),
		FinishedAt: time.Date(2026, 11, 13, 11, 2, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Explain(summary.ID)
	if err != nil || got.Signals[0] != "runner_label" {
		t.Fatalf("%+v %v", got, err)
	}
	logs, err := svc.JobLogs(summary.ID)
	if err != nil || !logs.Stored || !strings.Contains(logs.Log, "Waiting for a runner") {
		t.Fatalf("%+v %v", logs, err)
	}
}

func TestUsageMeter(t *testing.T) {
	svc := testService(t, nil)
	res, err := svc.Connect(context.Background(), ConnectInput{
		Account: "octocat", AccountKind: "user", Repos: []string{"octocat/hello"},
		Size:  "linux-x64-2",
		Facts: []RepoFact{{FullName: "octocat/hello", License: "MIT"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.IngestJob(IngestInput{
		Repo: "octocat/hello", Name: "build", Conclusion: "success",
		RunnerName: res.Repos[0].Runner.Name,
		StartedAt:  time.Date(2026, 11, 13, 11, 0, 0, 0, time.UTC),
		FinishedAt: time.Date(2026, 11, 13, 11, 1, 30, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	usage, err := svc.Usage("octocat", "2026-11")
	if err != nil {
		t.Fatal(err)
	}
	if len(usage.BySize) != 1 || usage.BySize[0].Seconds != 90 || usage.BySize[0].BillableSeconds != 30 {
		t.Fatalf("%+v", usage)
	}
	// 30s at $0.004/min = 30 * 4000 / 60 = 2000 micro-USD.
	if usage.BillableMicroUSD != 2000 || usage.Note == "" {
		t.Fatalf("%+v", usage)
	}
}

func TestSetRunsOnDryRunDoesNotApply(t *testing.T) {
	svc := testService(t, nil)
	plan, err := svc.SetRunsOn(context.Background(), RunsOnInput{
		Repo: "octocat/hello", Size: "linux-x64-2",
		Workflows: []WorkflowFile{{Path: ".github/workflows/ci.yml", Content: "on: push\njobs:\n  b:\n    runs-on: ubuntu-latest\n"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.DryRun || plan.PullURL != "" || len(plan.Changes) != 1 {
		t.Fatalf("%+v", plan)
	}
	if _, err := svc.SetRunsOn(context.Background(), RunsOnInput{
		Repo: "octocat/hello", Size: "linux-x64-2", Apply: true,
		Workflows: []WorkflowFile{{Path: ".github/workflows/ci.yml", Content: "on: push\njobs:\n  b:\n    runs-on: ubuntu-latest\n"}},
	}); err == nil {
		t.Fatal("apply without GitHub should fail")
	}
}

func TestSetRunsOnUnknownSize(t *testing.T) {
	svc := testService(t, nil)
	_, err := svc.SetRunsOn(context.Background(), RunsOnInput{
		Repo: "octocat/hello", Size: "nope",
		Workflows: []WorkflowFile{{Path: "a.yml", Content: "runs-on: ubuntu-latest\n"}},
	})
	if err == nil {
		t.Fatal("unknown size should fail")
	}
}

func TestLoadWardenFleetFile(t *testing.T) {
	dir := t.TempDir()
	body := `{"runners":[{"id":"runner-a","name":"runner-a","kind":"qemu","vmid":100,"host":"192.0.2.40","repos":["example/repo"]}]}`
	if err := os.WriteFile(filepath.Join(dir, "ci_runners.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	svc, err := Open(dir, Options{Provisioner: Unconfigured{}})
	if err != nil {
		t.Fatal(err)
	}
	health, err := svc.RunnerHealth(context.Background(), "runner-a")
	if err != nil {
		t.Fatal(err)
	}
	if health.GitHub.State != HealthUnknown || health.Substrate.State != HealthUnknown {
		t.Fatalf("%+v", health)
	}
	if health.Overall != HealthUnknown {
		t.Fatal(health.Overall)
	}
}

func TestWebhookRecordsInstallationAndJob(t *testing.T) {
	svc := testService(t, nil)
	_, err := svc.Connect(context.Background(), ConnectInput{
		Account: "octocat", AccountKind: "user", Repos: []string{"octocat/hello"},
		Size: "linux-x64-2", Facts: []RepoFact{{FullName: "octocat/hello", License: "MIT"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"action":"created","installation":{"id":9,"account":{"login":"octocat","type":"User"}}}`)
	if err := svc.HandleWebhook("installation", body); err != nil {
		t.Fatal(err)
	}
	job := []byte(`{"action":"completed","repository":{"full_name":"octocat/hello"},"workflow_job":{"id":42,"name":"build","conclusion":"success","started_at":"2026-11-13T11:00:00Z","completed_at":"2026-11-13T11:05:00Z"}}`)
	if err := svc.HandleWebhook("workflow_job", job); err != nil {
		t.Fatal(err)
	}
	jobs, err := svc.ListJobs("octocat/hello")
	if err != nil || len(jobs) != 1 || jobs[0].Conclusion != "success" || jobs[0].LogStored {
		t.Fatalf("%+v %v", jobs, err)
	}
	if err := svc.HandleWebhook("workflow_job", []byte(`{"action":"completed","repository":{"full_name":"other/nope"},"workflow_job":{"id":7,"conclusion":"failure"}}`)); err != nil {
		t.Fatal(err)
	}
	all, _ := svc.ListJobs("")
	if len(all) != 1 {
		t.Fatalf("unconnected repo was stored: %+v", all)
	}
}

func intPtr(n int) *int { return &n }

func TestVerifyWebhook(t *testing.T) {
	body := []byte(`{"ok":true}`)
	sig := "sha256=" + hmacSHA256([]byte("secret"), body)
	if !VerifyWebhook("secret", sig, body) {
		t.Fatal("valid signature rejected")
	}
	if VerifyWebhook("secret", sig, []byte(`{"ok":false}`)) || VerifyWebhook("", sig, body) {
		t.Fatal("invalid signature accepted")
	}
}
