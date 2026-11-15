package runner_job_test

import (
	"testing"
	"time"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/mcpl/mcpltest"
	"shippin.cloud/console/internal/runtime/runner_job"
)

const (
	acme   = "01a14149-0000-7000-8000-00000000000a"
	globex = "01a14149-0000-7000-8000-00000000000b"
	wsA    = "01a14149-0000-7000-8000-0000000000a1"
	wsB    = "01a14149-0000-7000-8000-0000000000b1"
)

// Two organizations on plans that include different runner sizes, each with its own GitHub
// installation and an assigned repository; Acme also has a repository it has not assigned.
const fixture = `
  INSERT INTO plans(id,label,rank,coverage) VALUES('builder','Builder',50,'foundational'), ('plain','Plain',51,'foundational');
  INSERT INTO plan_services(plan_id,service_id,enabled,lock,limits) VALUES
    ('builder','runners',true,'bounded','{"sizes":["2x"],"concurrent":2,"minutes":1}'),
    ('plain','runners',false,'locked','{}');
  INSERT INTO tenants(id,idp_org_id,name,plan,status) VALUES('` + acme + `','unlinked:a','Acme','builder','active'), ('` + globex + `','unlinked:b','Globex','plain','active');
  INSERT INTO shippin_workspaces(id,tenant_id,name,created_by,is_default) VALUES('` + wsA + `','` + acme + `','Main','ada',false), ('` + wsB + `','` + globex + `','Main','bob',false);
  INSERT INTO github_installations(id,tenant_id,account_login,account_type) VALUES(11,'` + acme + `','acme','Organization'), (22,'` + globex + `','globex','Organization');
  INSERT INTO shippin_github_repositories(id,tenant_id,installation_id,full_name,workspace_id,runners,updated_by) VALUES
    (101,'` + acme + `',11,'acme/app','` + wsA + `',true,'ada'),
    (103,'` + acme + `',11,'acme/docs','` + wsA + `',false,'ada'),
    (201,'` + globex + `',22,'globex/api','` + wsB + `',true,'bob');
  INSERT INTO shippin_runner_sizes(id,label,cores,memory_mib,disk_gib,image) VALUES('2x','2 cores',2,8192,40,'runner-ubuntu'), ('8x','8 cores',8,32768,80,'runner-ubuntu'),
    ('new','Not ready',2,8192,40,'');`

func TestReceive(t *testing.T) {
	h := mcpltest.Serve(t, actions.All, "shippin.runtime.runner_job.", mcpltest.As{Actor: "ada", Tenant: acme})
	if _, err := h.Pool.Exec(t.Context(), fixture); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	job := func(id int64, inst, repo int64, name string, labels ...string) runner_job.Webhook {
		return runner_job.Webhook{Action: "queued", InstallationID: inst, RepositoryID: repo, Repository: "x/y", JobID: id, RunID: 9, Name: name, Labels: labels}
	}
	receive := func(w runner_job.Webhook) runner_job.Outcome {
		t.Helper()
		out, err := runner_job.Receive(ctx, h.Pool, w)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}

	// Not ours: GitHub's own runners, and an installation no organization holds.
	if out := receive(job(1, 11, 101, "lint", "ubuntu-latest")); out.Ours {
		t.Fatalf("a job for GitHub's runners was taken: %+v", out)
	}
	if out := receive(job(2, 99, 101, "build", "shippin-2x")); out.Ours {
		t.Fatalf("an unknown installation's job was taken: %+v", out)
	}

	ok := receive(job(3, 11, 101, "build", "self-hosted", "Shippin-2X"))
	if ok.Status != runner_job.Queued || ok.Reason != "" || ok.JobID == "" {
		t.Fatalf("an allowed job waits for a runner: %+v", ok)
	}
	if again := receive(job(3, 11, 101, "build", "shippin-2x")); again.JobID != ok.JobID || again.Status != runner_job.Queued {
		t.Fatalf("a repeated job is recorded once: %+v", again)
	}
	for name, c := range map[string]struct {
		w    runner_job.Webhook
		says string
	}{
		"unassigned":   {job(4, 11, 104, "build", "shippin-2x"), "not assigned"},
		"not allowed":  {job(5, 11, 103, "build", "shippin-2x"), "not allowed on Shippin runners"},
		"two sizes":    {job(6, 11, 101, "build", "shippin-2x", "shippin-8x"), "more than one"},
		"unknown size": {job(7, 11, 101, "build", "shippin-64x"), "no Shippin runner size 64x"},
		"not ready":    {job(8, 11, 101, "build", "shippin-new"), "not ready yet"},
		"not on plan":  {job(9, 11, 101, "build", "shippin-8x"), "does not include runner size 8x"},
		"no runners":   {job(10, 22, 201, "build", "shippin-2x"), "does not include build runners"},
	} {
		out := receive(c.w)
		if out.Status != runner_job.Refused || !contains(out.Reason, c.says) {
			t.Errorf("%s: %+v, want refused saying %q", name, out, c.says)
		}
	}

	// It moves on: taken by a runner, then finished, with its time.
	start := time.Now().Add(-90 * time.Second).UTC().Truncate(time.Second)
	end := start.Add(75 * time.Second)
	if out := receive(runner_job.Webhook{Action: "in_progress", InstallationID: 11, RepositoryID: 101, JobID: 3, RunnerName: "shippin-acme-3", StartedAt: &start, Labels: []string{"shippin-2x"}}); out.Status != runner_job.InProgress {
		t.Fatalf("started: %+v", out)
	}
	// Globex's installation cannot move Acme's job.
	if out := receive(runner_job.Webhook{Action: "completed", InstallationID: 22, RepositoryID: 101, JobID: 3, Conclusion: "failure", Labels: []string{"shippin-2x"}}); out.JobID != "" {
		t.Fatalf("another organization's delivery changed Acme's job: %+v", out)
	}
	if out := receive(runner_job.Webhook{Action: "completed", InstallationID: 11, RepositoryID: 101, JobID: 3, Conclusion: "success", StartedAt: &start, CompletedAt: &end, Labels: []string{"shippin-2x"}}); out.Status != runner_job.Completed {
		t.Fatalf("finished: %+v", out)
	}
	// A refused job that GitHub later cancels stays refused, with its reason.
	if out := receive(runner_job.Webhook{Action: "completed", InstallationID: 11, RepositoryID: 104, JobID: 4, Conclusion: "cancelled", Labels: []string{"shippin-2x"}}); out.JobID != "" {
		t.Fatalf("a refused job moved on: %+v", out)
	}

	// The plan's one included minute is now used: the next job is refused.
	if out := receive(job(11, 11, 101, "build", "shippin-2x")); out.Status != runner_job.Refused || !contains(out.Reason, "minutes are used up") {
		t.Fatalf("over the minutes: %+v", out)
	}

	// Members see their own organization's jobs only.
	base := "/v1/tenants/" + acme + "/runtime/runner-jobs"
	items := h.Call("GET", base, "", 200)["items"].([]any)
	if len(items) != 8 {
		t.Fatalf("Acme's jobs: %d, want 8 (one allowed, six refused, one over the minutes)", len(items))
	}
	done := h.Call("GET", base+"/"+ok.JobID, "", 200)
	if done["status"] != "completed" || done["seconds"] != float64(75) || done["conclusion"] != "success" || done["runner_name"] != "shippin-acme-3" ||
		done["workspace_id"] != wsA || done["size"] != "2x" {
		t.Fatalf("the finished job: %v", done)
	}
	if n := len(h.Call("GET", base+"?workspace="+wsA, "", 200)["items"].([]any)); n != 7 {
		t.Fatalf("jobs in workspace Main: %d, want 7 (the unassigned one has none)", n)
	}
	h.As.Tenant = globex
	other := "/v1/tenants/" + globex + "/runtime/runner-jobs"
	if l := h.Call("GET", other, "", 200)["items"].([]any); len(l) != 1 {
		t.Fatalf("Globex's jobs: %v", l)
	}
	h.Call("GET", other+"/"+ok.JobID, "", 404)
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
