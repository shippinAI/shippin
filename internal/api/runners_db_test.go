package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/jobs"
	"shippin.cloud/console/internal/servers"
)

// The webhook route queues a start for a waiting job and a removal for the machine of one that ended;
// the runner hosts are the configured fleet's. The dispatch rules are tested in internal/runtime/runner.
func TestRunnerWebhookQueuesStartAndRemoval(t *testing.T) {
	a, d, tn, _ := githubAPI(t)
	ctx := context.Background()
	a.jobs = jobs.NewStore(d.Pool)
	a.runner = jobs.NewRunner(a.jobs, jobs.Settings{Workers: 1})
	a.cfg.Jobs = config.Jobs{Enabled: true, MaxAttempts: 3}
	fleet, host, other := uuid.NewString(), uuid.NewString(), uuid.NewString()
	a.cfg.Runners = config.Runners{Enabled: true, Fleet: fleet, JobLimitMinutes: 60}
	n := time.Now().UnixNano()%1_000_000_000 + 1
	inst, repo, job := n, n+1, n+2
	plan, ws := "runners-"+uuid.NewString()[:8], uuid.NewString()
	size := fmt.Sprintf("t%d", n)
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO plans(id,label,rank,coverage) VALUES($1,'Runners',900,'foundational')`, []any{plan}},
		{`INSERT INTO plan_services(plan_id,service_id,enabled,lock,limits) VALUES($1,'runners',true,'bounded',jsonb_build_object('sizes',jsonb_build_array($2::text),'concurrent',1,'minutes',0))`, []any{plan, size}},
		{`UPDATE tenants SET plan=$1 WHERE id=$2::uuid`, []any{plan, tn.ID}},
		{`INSERT INTO shippin_workspaces(id,tenant_id,name,created_by,is_default) VALUES($1::uuid,$2::uuid,'Main','ada',false)`, []any{ws, tn.ID}},
		{`INSERT INTO github_installations(id,tenant_id,account_login,account_type) VALUES($1,$2::uuid,'acme','Organization')`, []any{inst, tn.ID}},
		{`INSERT INTO shippin_github_repositories(id,tenant_id,installation_id,full_name,workspace_id,runners,updated_by) VALUES($1,$2::uuid,$3,'acme/app',$4::uuid,true,'ada')`, []any{repo, tn.ID, inst, ws}},
		{`INSERT INTO shippin_runner_sizes(id,label,cores,memory_mib,disk_gib,image) VALUES($1,'Test',2,4096,20,'runner')`, []any{size}},
		{`INSERT INTO infra_fleets(id,label,created_by) VALUES($1::uuid,'Runners','test')`, []any{fleet}},
		{`INSERT INTO servers(id,tenant_id,slug,label,site,state,created_by,hv_kind,fleet_id) VALUES
		    ($1::uuid,$3::uuid,'r-'||$1::text,'Runner host','main','planned','test','incus',$4::uuid),
		    ($2::uuid,$3::uuid,'o-'||$2::text,'Other host','main','planned','test','incus',NULL)`, []any{host, other, servers.SystemTenantID, fleet}},
	} {
		if _, err := d.Pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("%s: %v", q.sql, err)
		}
	}
	if hs, err := (machinePort{a}).Hosts(ctx); err != nil || len(hs) != 1 || hs[0] != host {
		t.Fatalf("runner hosts are the fleet's: %v %v", hs, err)
	}
	send := func(body string) {
		t.Helper()
		m := hmac.New(sha256.New, []byte("whs"))
		m.Write([]byte(body))
		r := httptest.NewRequest("POST", "/v1/github/webhook", strings.NewReader(body))
		r.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(m.Sum(nil)))
		r.Header.Set("X-GitHub-Delivery", uuid.NewString())
		r.Header.Set("X-GitHub-Event", "workflow_job")
		w := httptest.NewRecorder()
		a.githubWebhook(w, r)
		if w.Code != 204 {
			t.Fatalf("delivery: %d", w.Code)
		}
	}
	live := func(kind, target string) bool {
		var ok bool
		_ = d.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM jobs WHERE kind=$1 AND target_id=$2 AND tenant_id=$3::uuid AND status='queued')`, kind, target, tn.ID).Scan(&ok)
		return ok
	}
	send(fmt.Sprintf(`{"action":"queued","installation":{"id":%d},"repository":{"id":%d,"full_name":"acme/app"},
	  "workflow_job":{"id":%d,"run_id":5,"name":"build","labels":["shippin-%s"]}}`, inst, repo, job, size))
	var jobID string
	if err := d.Pool.QueryRow(ctx, `SELECT id::text FROM shippin_runner_jobs WHERE github_job_id=$1 AND status='queued'`, job).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	if !live(jobs.KindRunnerProvision, jobID) {
		t.Fatal("a waiting job's start was not queued")
	}
	machine := uuid.NewString()
	if _, err := d.Pool.Exec(ctx, `INSERT INTO shippin_runner_machines(id,tenant_id,runner_job_id,server_id,name,guest,size,attempt,state)
	  VALUES($1::uuid,$2::uuid,$3::uuid,$4::uuid,'shr-'||substr(md5($1::text),1,12),'runner-a',$5,1,'running')`, machine, tn.ID, jobID, host, size); err != nil {
		t.Fatal(err)
	}
	send(fmt.Sprintf(`{"action":"completed","installation":{"id":%d},"repository":{"id":%d,"full_name":"acme/app"},
	  "workflow_job":{"id":%d,"run_id":5,"name":"build","labels":["shippin-%s"],"runner_name":"runner-a","conclusion":"success"}}`, inst, repo, job, size))
	var state string
	_ = d.Pool.QueryRow(ctx, `SELECT state FROM shippin_runner_machines WHERE id=$1`, machine).Scan(&state)
	if state != "removing" || !live(jobs.KindRunnerRemove, machine) {
		t.Fatalf("the ended job's machine is %s, removal queued %v", state, live(jobs.KindRunnerRemove, machine))
	}

	// Runners off: nothing is queued.
	a.cfg.Runners.Enabled = false
	if hs, _ := (machinePort{a}).Hosts(ctx); len(hs) != 0 {
		t.Fatalf("hosts with runners off: %v", hs)
	}
}
