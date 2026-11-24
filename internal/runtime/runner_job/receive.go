package runner_job

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"shippin.cloud/console/internal/ext/limits"
	"shippin.cloud/console/internal/services/runner_size"
)

// Webhook is one workflow_job delivery from GitHub, already verified and deduplicated by the panel's
// GitHub route.
type Webhook struct {
	// Action is GitHub's: queued, waiting, in_progress or completed.
	Action         string
	InstallationID int64
	RepositoryID   int64
	Repository     string
	JobID          int64
	RunID          int64
	Name           string
	// Labels are the job's runs-on labels.
	Labels      []string
	RunnerName  string
	Conclusion  string
	StartedAt   *time.Time
	CompletedAt *time.Time
}

// Outcome is what Receive did with a delivery.
type Outcome struct {
	// Ours is false for a job that did not ask for a Shippin runner, or came through an installation
	// no organization holds; nothing is recorded for it.
	Ours   bool
	JobID  string
	Status Status
	Reason string
}

// Receive records what GitHub says about a job that asked for a Shippin runner, for the organization
// whose installation sent it. A queued job is checked against the repository's assignment, the size
// and the organization's plan, and is recorded as waiting for a runner or refused with the reason.
// Later deliveries move it on; they change only that organization's own job. The limits are
// limits.FromPlan; ReceiveUnder takes them from a policy.
func Receive(ctx context.Context, pool *pgxpool.Pool, w Webhook) (Outcome, error) {
	return ReceiveUnder(ctx, pool, limits.FromPlan{}, w)
}

// ReceiveUnder is Receive with the organization's limits taken from lim.
func ReceiveUnder(ctx context.Context, pool *pgxpool.Pool, lim limits.Policy, w Webhook) (Outcome, error) {
	var sizes []string
	for _, l := range w.Labels {
		if s, ok := strings.CutPrefix(strings.ToLower(l), runner_size.Prefix); ok {
			sizes = append(sizes, s)
		}
	}
	if len(sizes) == 0 || w.JobID <= 0 {
		return Outcome{}, nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return Outcome{}, err
	}
	defer tx.Rollback(ctx)
	var tenant string
	err = tx.QueryRow(ctx, `SELECT tenant_id::text FROM github_installations WHERE id=$1 AND deleted_at IS NULL`, w.InstallationID).Scan(&tenant)
	if errors.Is(err, pgx.ErrNoRows) {
		return Outcome{}, nil
	}
	if err != nil {
		return Outcome{}, err
	}
	out := Outcome{Ours: true}
	switch w.Action {
	case "queued":
		out, err = intake(ctx, tx, lim, tenant, w, sizes)
	case "in_progress":
		err = tx.QueryRow(ctx, `UPDATE shippin_runner_jobs SET status='in_progress', started_at=coalesce($3, now()), runner_name=left($4, 100), updated_at=now()
		  WHERE github_job_id=$1 AND tenant_id=$2 AND status='queued' RETURNING id::text, status`, w.JobID, tenant, w.StartedAt, w.RunnerName).Scan(&out.JobID, &out.Status)
	case "completed":
		err = tx.QueryRow(ctx, `UPDATE shippin_runner_jobs SET status = CASE WHEN $3 = 'cancelled' THEN 'canceled' ELSE 'completed' END,
		    conclusion=left($3, 40), started_at=coalesce(started_at, $4), completed_at=coalesce($5, now()), updated_at=now()
		  WHERE github_job_id=$1 AND tenant_id=$2 AND status IN ('queued','in_progress') RETURNING id::text, status`,
			w.JobID, tenant, w.Conclusion, w.StartedAt, w.CompletedAt).Scan(&out.JobID, &out.Status)
	default:
		return out, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		// A job that was refused, already finished, or never seen queued: nothing to move on.
		return out, nil
	}
	if err != nil {
		return Outcome{}, err
	}
	return out, tx.Commit(ctx)
}

// intake records a queued job, waiting or refused.
func intake(ctx context.Context, tx pgx.Tx, lim limits.Policy, tenant string, w Webhook, sizes []string) (Outcome, error) {
	out := Outcome{Ours: true, Status: Queued}
	var workspace, project *string
	var allowed bool
	err := tx.QueryRow(ctx, `SELECT workspace_id::text, project_id::text, runners FROM shippin_github_repositories WHERE id=$1 AND tenant_id=$2`,
		w.RepositoryID, tenant).Scan(&workspace, &project, &allowed)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	size := sizes[0]
	refuse := func(reason string) { out.Status, out.Reason = Refused, reason }
	switch {
	case len(sizes) > 1:
		refuse("The job asks for more than one Shippin runner size. Ask for one, such as runs-on: shippin-" + sizes[0] + ".")
	case workspace == nil:
		refuse("This repository is not assigned to a workspace. Assign it, and allow Shippin runners, under GitHub repositories.")
	case !allowed:
		refuse("This repository's jobs are not allowed on Shippin runners. Allow them under GitHub repositories.")
	default:
		reason, err := sizeAndPlan(ctx, tx, lim, tenant, *workspace, size)
		if err != nil {
			return out, err
		}
		if reason != "" {
			refuse(reason)
		}
	}
	err = tx.QueryRow(ctx, `INSERT INTO shippin_runner_jobs(id, github_job_id, run_id, tenant_id, repository_id, repository, workspace_id, project_id,
	    name, labels, size, status, reason)
	  VALUES($1, $2, $3, $4, $5, left($6, 140), $7::uuid, $8::uuid, left($9, 200), $10, $11, $12, $13)
	  ON CONFLICT (github_job_id) DO NOTHING RETURNING id::text`,
		uuid.Must(uuid.NewV7()), w.JobID, w.RunID, tenant, w.RepositoryID, w.Repository, workspace, project, w.Name, labels(w.Labels), size, out.Status, out.Reason).Scan(&out.JobID)
	if errors.Is(err, pgx.ErrNoRows) {
		// Already recorded: GitHub sent the job again under a new delivery.
		err = tx.QueryRow(ctx, `SELECT id::text, status, reason FROM shippin_runner_jobs WHERE github_job_id=$1 AND tenant_id=$2`, w.JobID, tenant).
			Scan(&out.JobID, &out.Status, &out.Reason)
	}
	return out, err
}

// sizeAndPlan is why the organization's limits or the size refuse the job, or "".
func sizeAndPlan(ctx context.Context, tx pgx.Tx, lim limits.Policy, tenant, workspace, size string) (string, error) {
	var active bool
	var image string
	err := tx.QueryRow(ctx, `SELECT active, image FROM shippin_runner_sizes WHERE id=$1`, size).Scan(&active, &image)
	switch {
	case errors.Is(err, pgx.ErrNoRows) || err == nil && !active:
		return "There is no Shippin runner size " + size + ". Ask for one your plan includes.", nil
	case err != nil:
		return "", err
	case image == "":
		return "Runner size " + size + " is not ready yet: the provider has not set its image.", nil
	}
	r, err := lim.Runners(ctx, tx, tenant, workspace)
	if err != nil {
		return "", err
	}
	if !r.Enabled {
		if r.Why == "" {
			return "This organization may not use build runners.", nil
		}
		return r.Why, nil
	}
	minutes := r.MonthlyMinutes
	if !slices.Contains(r.Sizes, size) {
		return "Your plan does not include runner size " + size + ".", nil
	}
	if minutes > 0 {
		var used float64
		if err := tx.QueryRow(ctx, `SELECT coalesce(sum(extract(epoch FROM completed_at - started_at)), 0) / 60 FROM shippin_runner_jobs
		  WHERE tenant_id=$1 AND started_at IS NOT NULL AND completed_at >= date_trunc('month', now())`, tenant).Scan(&used); err != nil {
			return "", err
		}
		if used >= minutes {
			return fmt.Sprintf("This month's %d included runner minutes are used up.", int(minutes)), nil
		}
	}
	return "", nil
}

// labels keeps at most 20 labels of up to 100 characters each.
func labels(in []string) []string {
	out := []string{}
	for _, l := range in {
		if len(out) == 20 {
			break
		}
		if len(l) > 100 {
			l = l[:100]
		}
		out = append(out, l)
	}
	return out
}
