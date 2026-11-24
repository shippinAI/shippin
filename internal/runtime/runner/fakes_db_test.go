package runner_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"shippin.cloud/console/internal/infrastructure/servers"
	"shippin.cloud/console/internal/mcpl/mcpltest"
	"shippin.cloud/console/internal/ports"
	"shippin.cloud/console/internal/runtime/runner"
	"shippin.cloud/console/internal/runtime/runner_job"
)

const (
	acme    = "01a14149-0000-7000-8000-00000000000a"
	globex  = "01a14149-0000-7000-8000-00000000000b"
	initech = "01a14149-0000-7000-8000-00000000000c"
	wsA     = "01a14149-0000-7000-8000-0000000000a1"
	wsB     = "01a14149-0000-7000-8000-0000000000b1"
	wsC     = "01a14149-0000-7000-8000-0000000000c1"
	host1   = "01a14149-0000-7000-8000-0000000000f1"
	host2   = "01a14149-0000-7000-8000-0000000000f2"
)

// Three organizations whose plans run two, one and no runner machines at once, each with its own
// installation and an assigned repository allowed on runners; two provider hosts.
const fixture = `
  INSERT INTO plans(id,label,rank,coverage) VALUES('builder','Builder',50,'foundational'), ('team','Team',51,'foundational'), ('zero','Zero',52,'foundational');
  INSERT INTO plan_services(plan_id,service_id,enabled,lock,limits) VALUES
    ('builder','runners',true,'bounded','{"sizes":["2x"],"concurrent":2,"minutes":0}'),
    ('team','runners',true,'bounded','{"sizes":["2x"],"concurrent":1,"minutes":0}'),
    ('zero','runners',true,'bounded','{"sizes":["2x"],"concurrent":0,"minutes":0}');
  INSERT INTO tenants(id,idp_org_id,name,plan,status) VALUES('` + acme + `','unlinked:a','Acme','builder','active'),
    ('` + globex + `','unlinked:b','Globex','team','active'), ('` + initech + `','unlinked:c','Initech','zero','active');
  INSERT INTO shippin_workspaces(id,tenant_id,name,created_by,is_default) VALUES('` + wsA + `','` + acme + `','Main','ada',false),
    ('` + wsB + `','` + globex + `','Main','bob',false), ('` + wsC + `','` + initech + `','Main','cy',false);
  INSERT INTO github_installations(id,tenant_id,account_login,account_type) VALUES(11,'` + acme + `','acme','Organization'),
    (22,'` + globex + `','globex','Organization'), (33,'` + initech + `','initech','Organization');
  INSERT INTO shippin_github_repositories(id,tenant_id,installation_id,full_name,workspace_id,runners,updated_by) VALUES
    (101,'` + acme + `',11,'acme/app','` + wsA + `',true,'ada'),
    (201,'` + globex + `',22,'globex/api','` + wsB + `',true,'bob'),
    (301,'` + initech + `',33,'initech/tps','` + wsC + `',true,'cy');
  INSERT INTO shippin_runner_sizes(id,label,cores,memory_mib,disk_gib,image) VALUES('2x','2 cores',2,8192,40,'runner-ubuntu');
  INSERT INTO servers(id,tenant_id,slug,label,site,state,created_by,hv_kind) VALUES
    ('` + host1 + `','` + servers.SystemTenantID + `','runners-1','Runners 1','main','planned','test','incus'),
    ('` + host2 + `','` + servers.SystemTenantID + `','runners-2','Runners 2','main','planned','test','incus');`

// world is a test database with the fixture, a fake GitHub and fake hosts.
type world struct {
	t    *testing.T
	pool *pgxpool.Pool
	gh   *github
	m    *hosts
	d    runner.Deps
	// installation and repository of each organization
	repo map[string][2]int64
}

func newWorld(t *testing.T, pool *pgxpool.Pool) *world {
	t.Helper()
	if pool == nil {
		pool = mcpltest.DB(t)
	}
	if _, err := pool.Exec(t.Context(), fixture); err != nil {
		t.Fatal(err)
	}
	w := &world{t: t, pool: pool, gh: &github{}, m: &hosts{hosts: []string{host1, host2}, made: map[string]ports.Machine{}, on: map[string]string{}},
		repo: map[string][2]int64{acme: {11, 101}, globex: {22, 201}, initech: {33, 301}}}
	w.d = runner.Deps{Pool: pool, GitHub: w.gh, Machines: w.m, JobLimit: time.Hour}
	return w
}

// queue records GitHub job n of an organization as waiting, n seconds into the day, and answers its id.
func (w *world) queue(tenant string, n int64) string {
	w.t.Helper()
	r := w.repo[tenant]
	out, err := runner_job.Receive(w.t.Context(), w.pool, runner_job.Webhook{Action: "queued", InstallationID: r[0], RepositoryID: r[1],
		Repository: map[string]string{acme: "acme/app", globex: "globex/api", initech: "initech/tps"}[tenant], JobID: n, RunID: 1, Name: "build",
		Labels: []string{"self-hosted", "shippin-2x"}})
	if err != nil || out.Status != runner_job.Queued {
		w.t.Fatalf("queue %d: %+v %v", n, out, err)
	}
	if _, err := w.pool.Exec(w.t.Context(), `UPDATE shippin_runner_jobs SET queued_at = '2026-01-01T00:00:00Z'::timestamptz + make_interval(secs => $2) WHERE id=$1`, out.JobID, n); err != nil {
		w.t.Fatal(err)
	}
	return out.JobID
}

// receive sends a later delivery for GitHub job n and follows it as the panel does.
func (w *world) receive(tenant, action string, n int64, runnerName, conclusion string) (runner_job.Outcome, []string) {
	w.t.Helper()
	r := w.repo[tenant]
	out, err := runner_job.Receive(w.t.Context(), w.pool, runner_job.Webhook{Action: action, InstallationID: r[0], RepositoryID: r[1],
		JobID: n, RunnerName: runnerName, Conclusion: conclusion, Labels: []string{"shippin-2x"}})
	if err != nil {
		w.t.Fatal(err)
	}
	if out.JobID == "" {
		return out, nil
	}
	ids, err := runner.JobMoved(w.t.Context(), w.pool, out.JobID, string(out.Status), runnerName)
	if err != nil {
		w.t.Fatal(err)
	}
	return out, ids
}

func (w *world) provision(job string, final bool) runner.Outcome {
	w.t.Helper()
	out, err := runner.Provision(w.t.Context(), w.d, job, final)
	if err != nil {
		w.t.Fatalf("provision %s: %v", job, err)
	}
	return out
}

// machine is one machine's row: its id, state, host, guest and error.
type machine struct{ id, state, server, guest, err, reason string }

func (w *world) machineFor(job string) machine {
	w.t.Helper()
	var m machine
	if err := w.pool.QueryRow(w.t.Context(), `SELECT id::text, state, coalesce(server_id::text,''), guest, error, removal_reason FROM shippin_runner_machines WHERE runner_job_id::text=$1`, job).
		Scan(&m.id, &m.state, &m.server, &m.guest, &m.err, &m.reason); err != nil {
		w.t.Fatalf("machine for %s: %v", job, err)
	}
	return m
}

func (w *world) machines() int {
	var n int
	_ = w.pool.QueryRow(w.t.Context(), `SELECT count(*) FROM shippin_runner_machines`).Scan(&n)
	return n
}

// github is a fake GitHub App that registers just-in-time runners.
type github struct {
	mu    sync.Mutex
	calls []jit
	fail  error
}

type jit struct {
	inst, repo int64
	full, name string
	labels     []string
	config     string
}

func (g *github) Repositories(context.Context, int64) ([]ports.GitHubRepository, error) {
	return nil, errors.New("not used here")
}

func (g *github) JITConfig(_ context.Context, inst, repo int64, full, name string, labels []string) (string, int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.fail != nil {
		return "", 0, g.fail
	}
	c := jit{inst, repo, full, name, slices.Clone(labels), fmt.Sprintf("SeCrEtJiTcOnFiG%04d", len(g.calls)+1)}
	g.calls = append(g.calls, c)
	return c.config, int64(9000 + len(g.calls)), nil
}

// hosts is a fake set of provider hosts: what was made where, and what was deleted.
type hosts struct {
	mu         sync.Mutex
	hosts      []string
	made       map[string]ports.Machine // by guest
	on         map[string]string        // guest -> server, while it exists
	creates    int
	deleted    []string
	failCreate error
	failDelete error
	// hold, when set, makes Create signal entered and wait for release.
	hold             bool
	entered, release chan struct{}
}

func (h *hosts) Hosts(context.Context) ([]string, error) { return h.hosts, nil }

func (h *hosts) Create(_ context.Context, server string, m ports.Machine) (string, error) {
	h.mu.Lock()
	h.creates++
	hold, fail := h.hold, h.failCreate
	h.mu.Unlock()
	if hold {
		h.entered <- struct{}{}
		<-h.release
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.made[m.Name] = m
	h.on[m.Name] = server
	if fail != nil {
		// Like a hypervisor that made it but could not start it, and echoed what it was sent.
		return "", fmt.Errorf("%w: %s", fail, m.UserData)
	}
	return m.Name, nil
}

func (h *hosts) Delete(_ context.Context, server, guest string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.failDelete != nil {
		return h.failDelete
	}
	if h.on[guest] != "" && h.on[guest] != server {
		return fmt.Errorf("%s is not on %s", guest, server)
	}
	delete(h.on, guest)
	h.deleted = append(h.deleted, guest)
	return nil
}

// enqueued is a fake durable queue that records what was asked for, in order.
type enqueued struct {
	mu   sync.Mutex
	jobs []string // kind tenant target
}

func (e *enqueued) add(_ context.Context, kind, tenant, target string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.jobs = append(e.jobs, kind+" "+tenant+" "+target)
	return nil
}
