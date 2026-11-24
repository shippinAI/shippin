package runner

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"shippin.cloud/console/internal/ext/limits"
	"shippin.cloud/console/internal/ports"
	"shippin.cloud/console/internal/services/jobs"
	"shippin.cloud/console/internal/services/runner_size"
)

// startWindow is how long one attempt to start a machine may take, image download included. The
// attempt holds the machine until then, so a second run of the same job does not start it again.
const startWindow = 15 * time.Minute

// Enqueue adds a durable job of a kind for an organization and target, joining a live one.
type Enqueue func(ctx context.Context, kind, tenant, target string) error

// Deps are what dispatch needs: the database, the GitHub App and the provider's hosts.
type Deps struct {
	Pool     *pgxpool.Pool
	GitHub   ports.GitHub
	Machines ports.Machines
	// JobLimit is how long a machine may live. One older than this is removed whatever its job says.
	JobLimit time.Duration
	// Now is the clock; nil is time.Now. Only placement and logging read it; the database's now() decides ages.
	Now func() time.Time
	// Limits says how many machines an organization may have at once; nil is limits.FromPlan.
	Limits limits.Policy
}

func (d Deps) limits() limits.Policy {
	if d.Limits == nil {
		return limits.FromPlan{}
	}
	return d.Limits
}

// Sweep is the periodic pass: it removes machines older than the job time limit, retries removals
// that failed or were cut short, and starts waiting jobs where their organizations have room.
func Sweep(ctx context.Context, d Deps, enqueue Enqueue) error {
	var errs []error
	if d.JobLimit > 0 {
		if _, err := d.Pool.Exec(ctx, `UPDATE shippin_runner_machines SET state='removing', updated_at=now(),
		    removal_reason='It ran longer than the job time limit of '||$2||'.'
		  WHERE state IN ('starting','running') AND created_at < now() - make_interval(secs => $1)`, d.JobLimit.Seconds(), d.JobLimit.String()); err != nil {
			errs = append(errs, err)
		}
	}
	rows, err := d.Pool.Query(ctx, `SELECT id::text, tenant_id::text FROM shippin_runner_machines WHERE state IN ('removing','failed') ORDER BY updated_at`)
	if err != nil {
		return errors.Join(append(errs, err)...)
	}
	type pair struct{ id, tenant string }
	var gone []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.id, &p.tenant); err != nil {
			rows.Close()
			return errors.Join(append(errs, err)...)
		}
		gone = append(gone, p)
	}
	rows.Close()
	for _, p := range gone {
		if err := enqueue(ctx, jobs.KindRunnerRemove, p.tenant, p.id); err != nil {
			errs = append(errs, err)
		}
	}
	if err := StartWaitingUnder(ctx, d.Pool, d.limits(), enqueue, ""); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// StartWaiting queues a start for the waiting jobs each organization has room for, oldest first, taking
// one job from each organization in turn so one organization's backlog never holds back another's. An
// empty tenant means every organization. Provision checks the room again, so queuing too many only
// costs a job that waits. The limits are limits.FromPlan; StartWaitingUnder takes them from a policy.
func StartWaiting(ctx context.Context, pool *pgxpool.Pool, enqueue Enqueue, tenant string) error {
	return StartWaitingUnder(ctx, pool, limits.FromPlan{}, enqueue, tenant)
}

// StartWaitingUnder is StartWaiting with each organization's room taken from lim.
func StartWaitingUnder(ctx context.Context, pool *pgxpool.Pool, lim limits.Policy, enqueue Enqueue, tenant string) error {
	rows, err := pool.Query(ctx, `WITH waiting AS (
	    SELECT j.id, j.tenant_id, j.queued_at, row_number() OVER (PARTITION BY j.tenant_id ORDER BY j.queued_at, j.id) AS rank
	    FROM shippin_runner_jobs j
	    WHERE j.status = 'queued' AND ($1 = '' OR j.tenant_id::text = $1)
	      AND NOT EXISTS (SELECT 1 FROM shippin_runner_machines m WHERE m.runner_job_id = j.id))
	  SELECT w.id::text, w.tenant_id::text, w.rank,
	    (SELECT count(*) FROM shippin_runner_machines m WHERE m.tenant_id = w.tenant_id AND m.state <> 'removed')
	  FROM waiting w WHERE w.rank <= 200 ORDER BY w.rank, w.queued_at, w.id`, tenant)
	if err != nil {
		return err
	}
	type cand struct {
		job, tenant string
		rank, live  int
	}
	var cs []cand
	for rows.Next() {
		var c cand
		if err := rows.Scan(&c.job, &c.tenant, &c.rank, &c.live); err != nil {
			rows.Close()
			return err
		}
		cs = append(cs, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	limits := map[string]int{}
	var errs []error
	for _, c := range cs {
		limit, ok := limits[c.tenant]
		if !ok {
			if limit, err = Limit(ctx, lim, pool, c.tenant); err != nil {
				errs = append(errs, err)
				limit = 0
			}
			limits[c.tenant] = limit
		}
		if c.live+c.rank > limit {
			continue
		}
		if err := enqueue(ctx, jobs.KindRunnerProvision, c.tenant, c.job); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Limit is how many machines the organization may have at once under lim, or 0 when it may not use
// build runners.
func Limit(ctx context.Context, lim limits.Policy, q limits.Querier, tenant string) (int, error) {
	r, err := lim.Runners(ctx, q, tenant, "")
	if err != nil {
		return 0, err
	}
	if !r.Enabled || r.Concurrent < 1 {
		return 0, nil
	}
	return r.Concurrent, nil
}

// Outcome is what one Provision did, for the job's result. It never carries the runner configuration.
type Outcome struct {
	Started string `json:"started,omitempty"` // the machine started
	Waiting string `json:"waiting,omitempty"` // why the job still waits
	Skipped string `json:"skipped,omitempty"` // why there was nothing to do
}

// plan is what one attempt to start a machine needs, read under the organization's lock.
type plan struct {
	tenant, job, size   string
	repoID, inst        int64
	repo                string
	labels              []string
	machine, server     string
	guest, prior        string
	attempt             int
	cores, memory, disk int
	image               string
}

// Provision starts the machine for one waiting runner job, when its organization has room for it in
// arrival order: the job starts only if fewer than its plan's limit are running or starting and no
// earlier waiting job of the organization would be passed over. One machine is ever made for a job,
// so running this twice never starts it twice; a run after a failed or abandoned attempt carries on
// with the same machine. final says this is the job's last attempt: a failure then leaves the machine
// failed, for removal, and the job without a runner.
func Provision(ctx context.Context, d Deps, jobID string, final bool) (Outcome, error) {
	hosts, err := d.Machines.Hosts(ctx)
	if err != nil {
		return Outcome{}, err
	}
	p, out, err := reserve(ctx, d, jobID, hosts)
	if err != nil || out != (Outcome{}) {
		return out, err
	}
	actx, cancel := context.WithTimeout(ctx, startWindow)
	defer cancel()
	config := ""
	fail := func(err error) (Outcome, error) {
		msg := err.Error()
		if config != "" {
			msg = strings.ReplaceAll(msg, config, "[runner configuration]")
		}
		state := Starting
		if final {
			state = Failed
		}
		// Let a later run carry on (busy_until cleared), or leave it failed for removal.
		if _, e := d.Pool.Exec(context.WithoutCancel(ctx), `UPDATE shippin_runner_machines SET state=$3, error=left($4, 500), busy_until=NULL, updated_at=now()
		  WHERE id=$1 AND attempt=$2 AND state='starting'`, p.machine, p.attempt, state, "Starting it failed: "+msg); e != nil {
			return Outcome{}, errors.Join(errors.New(msg), e)
		}
		if final {
			_, _ = d.Pool.Exec(context.WithoutCancel(ctx), `UPDATE shippin_runner_jobs SET reason='Shippin could not start a runner for this job. Staff can see why.', updated_at=now()
			  WHERE id=$1 AND tenant_id=$2 AND status='queued'`, p.job, p.tenant)
		}
		return Outcome{}, errors.New(msg)
	}
	if p.prior != "" {
		// An earlier attempt may have left its machine behind; it never got a job, so it goes first.
		if err := d.Machines.Delete(actx, p.server, p.prior); err != nil {
			return fail(fmt.Errorf("removing the earlier attempt %s: %w", p.prior, err))
		}
	}
	config, runnerID, err := d.GitHub.JITConfig(actx, p.inst, p.repoID, p.repo, p.guest, jitLabels(p.labels, p.size))
	if err != nil {
		return fail(fmt.Errorf("registering the runner with GitHub: %w", err))
	}
	ud, err := UserData(config)
	if err != nil {
		return fail(err)
	}
	if _, err := d.Pool.Exec(ctx, `UPDATE shippin_runner_machines SET github_runner_id=$3, updated_at=now() WHERE id=$1 AND attempt=$2`, p.machine, p.attempt, runnerID); err != nil {
		return fail(err)
	}
	guest, err := d.Machines.Create(actx, p.server, ports.Machine{Name: p.guest, Image: p.image, Cores: p.cores, MemoryMiB: p.memory, DiskGiB: p.disk, UserData: ud})
	if err != nil {
		return fail(fmt.Errorf("making the machine: %w", err))
	}
	if guest == "" {
		guest = p.guest
	}
	tag, err := d.Pool.Exec(ctx, `UPDATE shippin_runner_machines SET state='running', guest=$3, started_at=now(), error='', busy_until=NULL, updated_at=now()
	  WHERE id=$1 AND attempt=$2 AND state='starting'`, p.machine, p.attempt, guest)
	if err != nil {
		return fail(err)
	}
	if tag.RowsAffected() == 0 {
		// It was marked for removal (its job ended, it was too old, or staff removed it) while it was
		// being made, and a removal may already have found nothing to delete: remove what was made.
		if err := d.Machines.Delete(context.WithoutCancel(ctx), p.server, guest); err != nil {
			_, _ = d.Pool.Exec(context.WithoutCancel(ctx), `UPDATE shippin_runner_machines SET state='failed', guest=$2, error=left($3, 500), updated_at=now()
			  WHERE id=$1 AND state IN ('removing','removed')`, p.machine, guest, "Removing it failed: "+err.Error())
			return Outcome{}, err
		}
		return Outcome{Skipped: "removed while it was being made"}, nil
	}
	return Outcome{Started: p.machine}, nil
}

// reserve decides, under the organization's lock, whether the job starts now and on which host, and
// records the machine before anything is made. A non-empty Outcome means there is nothing to make.
func reserve(ctx context.Context, d Deps, jobID string, hosts []string) (plan, Outcome, error) {
	var p plan
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return p, Outcome{}, err
	}
	defer tx.Rollback(ctx)
	var status string
	var queuedAt time.Time
	var size *string
	err = tx.QueryRow(ctx, `SELECT tenant_id::text, status, size, repository_id, repository, labels, queued_at FROM shippin_runner_jobs WHERE id::text=$1`, jobID).
		Scan(&p.tenant, &status, &size, &p.repoID, &p.repo, &p.labels, &queuedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, Outcome{Skipped: "no such runner job"}, nil
	}
	if err != nil {
		return p, Outcome{}, err
	}
	p.job = jobID
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('shippin.runner.' || $1, 0))`, p.tenant); err != nil {
		return p, Outcome{}, err
	}
	// Read again under the lock: a delivery may have moved the job on meanwhile.
	if err := tx.QueryRow(ctx, `SELECT status FROM shippin_runner_jobs WHERE id::text=$1`, jobID).Scan(&status); err != nil {
		return p, Outcome{}, err
	}
	var state State
	var busy bool
	var server *string
	err = tx.QueryRow(ctx, `SELECT id::text, state, attempt, guest, server_id::text, coalesce(busy_until > now(), false) FROM shippin_runner_machines WHERE runner_job_id::text=$1`, jobID).
		Scan(&p.machine, &state, &p.attempt, &p.prior, &server, &busy)
	switch {
	case err == nil && state != Starting:
		return p, Outcome{Skipped: "its machine is already " + string(state)}, nil
	case err == nil && busy:
		return p, Outcome{Skipped: "another attempt is starting its machine"}, nil
	case err == nil && server == nil:
		return p, Outcome{}, errors.New("runner machine " + p.machine + " has no host")
	case err == nil:
		p.server = *server // carry on where the last attempt stopped
	case !errors.Is(err, pgx.ErrNoRows):
		return p, Outcome{}, err
	case status != "queued" || size == nil:
		return p, Outcome{Skipped: "the job is " + status}, nil
	default:
		out, err := room(ctx, tx, d.limits(), p.tenant, jobID, queuedAt)
		if err != nil || out != (Outcome{}) {
			return p, out, err
		}
		if p.server, err = place(ctx, tx, hosts); err != nil || p.server == "" {
			return p, Outcome{Waiting: "no host is set up for runner machines"}, err
		}
		p.machine, p.prior = uuid.Must(uuid.NewV7()).String(), ""
	}
	if size == nil {
		return p, Outcome{Skipped: "the job has no size"}, nil
	}
	p.size = *size
	refuse := func(reason string) (plan, Outcome, error) {
		if p.attempt > 0 {
			return p, Outcome{}, errors.New(reason)
		}
		if _, err := tx.Exec(ctx, `UPDATE shippin_runner_jobs SET status='refused', reason=$3, updated_at=now() WHERE id::text=$1 AND tenant_id=$2`, jobID, p.tenant, reason); err != nil {
			return p, Outcome{}, err
		}
		return p, Outcome{Skipped: reason}, tx.Commit(ctx)
	}
	var active, allowed bool
	err = tx.QueryRow(ctx, `SELECT active, image, cores, memory_mib, disk_gib FROM shippin_runner_sizes WHERE id=$1`, p.size).Scan(&active, &p.image, &p.cores, &p.memory, &p.disk)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && (!active || p.image == "") {
		return refuse("Runner size " + p.size + " was removed or turned off before the job started.")
	}
	if err != nil {
		return p, Outcome{}, err
	}
	err = tx.QueryRow(ctx, `SELECT installation_id, runners FROM shippin_github_repositories WHERE id=$1 AND tenant_id=$2`, p.repoID, p.tenant).Scan(&p.inst, &allowed)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !allowed {
		return refuse("This repository's jobs were no longer allowed on Shippin runners when the job was to start.")
	}
	if err != nil {
		return p, Outcome{}, err
	}
	p.attempt++
	if p.attempt == 1 {
		name := "shr-" + randomHex(6)
		p.guest = name + "-1"
		_, err = tx.Exec(ctx, `INSERT INTO shippin_runner_machines(id, tenant_id, runner_job_id, server_id, name, guest, size, attempt, state, busy_until)
		  VALUES($1, $2, $3, $4, $5, $6, $7, 1, 'starting', now() + make_interval(secs => $8))`,
			p.machine, p.tenant, jobID, p.server, name, p.guest, p.size, startWindow.Seconds())
	} else {
		var name string
		if err = tx.QueryRow(ctx, `SELECT name FROM shippin_runner_machines WHERE id=$1`, p.machine).Scan(&name); err == nil {
			p.guest = name + "-" + strconv.Itoa(p.attempt)
			_, err = tx.Exec(ctx, `UPDATE shippin_runner_machines SET attempt=$2, guest=$3, busy_until=now() + make_interval(secs => $4), updated_at=now()
			  WHERE id=$1`, p.machine, p.attempt, p.guest, startWindow.Seconds())
		}
	}
	if err != nil {
		return p, Outcome{}, err
	}
	return p, Outcome{}, tx.Commit(ctx)
}

// room says why the job waits, or nothing when it may start now.
func room(ctx context.Context, tx pgx.Tx, lim limits.Policy, tenant, jobID string, queuedAt time.Time) (Outcome, error) {
	limit, err := Limit(ctx, lim, tx, tenant)
	if err != nil {
		return Outcome{}, err
	}
	if limit == 0 {
		return Outcome{Waiting: "the plan allows no runner machines at once"}, nil
	}
	var live, earlier int
	if err := tx.QueryRow(ctx, `SELECT
	    (SELECT count(*) FROM shippin_runner_machines WHERE tenant_id=$1 AND state <> 'removed'),
	    (SELECT count(*) FROM shippin_runner_jobs j WHERE j.tenant_id=$1 AND j.status='queued' AND j.size IS NOT NULL
	       AND (j.queued_at, j.id::text) < ($2, $3)
	       AND NOT EXISTS (SELECT 1 FROM shippin_runner_machines m WHERE m.runner_job_id=j.id))`, tenant, queuedAt, jobID).Scan(&live, &earlier); err != nil {
		return Outcome{}, err
	}
	switch {
	case live >= limit:
		return Outcome{Waiting: fmt.Sprintf("the organization has %d of its %d runner machines", live, limit)}, nil
	case live+earlier >= limit:
		return Outcome{Waiting: "earlier jobs of the organization start first"}, nil
	}
	return Outcome{}, nil
}

// place picks the host with the fewest machines not yet removed, in the hosts' order on a tie.
func place(ctx context.Context, tx pgx.Tx, hosts []string) (string, error) {
	if len(hosts) == 0 {
		return "", nil
	}
	load := map[string]int{}
	rows, err := tx.Query(ctx, `SELECT server_id::text, count(*) FROM shippin_runner_machines WHERE state <> 'removed' AND server_id::text = ANY($1) GROUP BY server_id`, hosts)
	if err != nil {
		return "", err
	}
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			rows.Close()
			return "", err
		}
		load[s] = n
	}
	rows.Close()
	best := hosts[0]
	for _, h := range hosts[1:] {
		if load[h] < load[best] {
			best = h
		}
	}
	return best, rows.Err()
}

// Remove deletes a machine marked for removal from its host and records it removed. A failed delete
// leaves it failed with the error; the sweep tries again. It answers the machine's organization, so
// the caller can start what was waiting for the room.
func Remove(ctx context.Context, d Deps, id string) (tenant string, err error) {
	var state State
	var server *string
	var guest string
	err = d.Pool.QueryRow(ctx, `SELECT tenant_id::text, state, server_id::text, guest FROM shippin_runner_machines WHERE id::text=$1`, id).Scan(&tenant, &state, &server, &guest)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil || (state != Removing && state != Failed) {
		return tenant, err
	}
	if server != nil && guest != "" {
		if err := d.Machines.Delete(ctx, *server, guest); err != nil {
			_, e := d.Pool.Exec(context.WithoutCancel(ctx), `UPDATE shippin_runner_machines SET state='failed', error=left($2, 500), updated_at=now()
			  WHERE id::text=$1 AND state IN ('removing','failed')`, id, "Removing it failed: "+err.Error())
			return tenant, errors.Join(err, e)
		}
	}
	_, err = d.Pool.Exec(ctx, `UPDATE shippin_runner_machines SET state='removed', removed_at=now(), busy_until=NULL, updated_at=now()
	  WHERE id::text=$1 AND state IN ('removing','failed')`, id)
	return tenant, err
}

// JobMoved follows a runner job GitHub moved on, after runner_job.Receive recorded it. When a runner
// takes a job, its machine is marked with the job it took. When a job ends, the machine that ran it is
// marked for removal, and so is the machine started for it if its runner never took a job; the
// answer is the machines to remove. Only the job's own organization's machines are touched.
func JobMoved(ctx context.Context, pool *pgxpool.Pool, jobID, status, runnerName string) ([]string, error) {
	var tenant string
	if err := pool.QueryRow(ctx, `SELECT tenant_id::text FROM shippin_runner_jobs WHERE id::text=$1`, jobID).Scan(&tenant); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	switch status {
	case "in_progress":
		if runnerName != "" {
			_, err := pool.Exec(ctx, `UPDATE shippin_runner_machines SET assigned_job_id=$3::uuid, updated_at=now()
			  WHERE tenant_id::text=$1 AND guest=$2 AND state <> 'removed'`, tenant, runnerName, jobID)
			return nil, err
		}
		return nil, nil
	case "completed", "canceled":
	default:
		return nil, nil
	}
	rows, err := pool.Query(ctx, `UPDATE shippin_runner_machines SET state='removing', removal_reason='Its job ended.', updated_at=now()
	  WHERE tenant_id::text=$1 AND state IN ('starting','running')
	    AND (($2 <> '' AND guest=$2) OR (runner_job_id::text=$3 AND (assigned_job_id IS NULL OR assigned_job_id::text=$3)))
	  RETURNING id::text`, tenant, runnerName, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// githubDefault are the labels GitHub gives every self-hosted runner itself; a just-in-time runner is
// registered with the job's other labels.
var githubDefault = []string{"self-hosted", "linux", "x64", "arm64", "arm", "windows", "macos"}

func jitLabels(labels []string, size string) []string {
	var out []string
	for _, l := range labels {
		if !slices.Contains(githubDefault, strings.ToLower(l)) && !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	if len(out) == 0 {
		out = []string{runner_size.Prefix + size}
	}
	return out
}

// configShape is what GitHub's encoded runner configuration looks like (base64). Anything else is
// refused before it reaches a machine's user data.
var configShape = regexp.MustCompile(`^[A-Za-z0-9+/=_-]+$`)

// RunnerDir and RunnerUser are where a runner image keeps the GitHub Actions runner and the user it
// runs as (docs/runners.md, Images).
const (
	RunnerDir  = "/opt/actions-runner"
	RunnerUser = "runner"
)

// UserData is the cloud-init user data that runs the runner once with this configuration and then
// powers the machine off. The configuration is written root-only and removed once read.
func UserData(config string) (string, error) {
	if len(config) > 64<<10 || !configShape.MatchString(config) {
		return "", errors.New("GitHub answered with a runner configuration of an unexpected shape")
	}
	quoted, _ := json.Marshal(config) // a JSON string is a YAML double-quoted scalar
	return `#cloud-config
# A Shippin build runner: it takes one job and powers off. The configuration is single-use.
write_files:
  - path: /run/shippin-runner/config
    permissions: "0600"
    content: ` + string(quoted) + `
  - path: /run/shippin-runner/run-once
    permissions: "0700"
    content: |
      #!/bin/sh
      echo "shippin-runner: starting the runner for one job"
      config=$(cat /run/shippin-runner/config)
      rm -f /run/shippin-runner/config
      cd ` + RunnerDir + ` && runuser -u ` + RunnerUser + ` -- ./run.sh --jitconfig "$config"
      echo "shippin-runner: finished ($?), powering off"
      poweroff
runcmd:
  - [sh, /run/shippin-runner/run-once]
`, nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
