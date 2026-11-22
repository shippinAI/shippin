// Package runner_job is an organization's build jobs: each GitHub Actions job that asked for a
// Shippin runner, with where it came from, the size it asked for and what happened (RUN-01,
// docs/runners.md). GitHub reports jobs through the panel's one webhook route, which hands them to
// Receive; members read them through list and get.
package runner_job

import (
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"shippin.cloud/console/internal/mcpl"
)

// Status is where a job stands.
type Status string

// Values are the statuses: waiting for a runner, refused (with the reason), running, done, or canceled.
func (Status) Values() []string {
	return []string{"queued", "refused", "in_progress", "completed", "canceled"}
}

const (
	Queued     Status = "queued"
	Refused    Status = "refused"
	InProgress Status = "in_progress"
	Completed  Status = "completed"
	Canceled   Status = "canceled"
)

// Job is one runner job as the organization reads it.
type Job struct {
	ID           string `json:"id"`
	GitHubJobID  string `json:"github_job_id"`
	RunID        string `json:"run_id"`
	RepositoryID string `json:"repository_id"`
	Repository   string `json:"repository"`
	// WorkspaceID and ProjectID are where the repository was assigned when the job came; empty for a
	// job refused because it was not assigned, or once that workspace or project is gone.
	WorkspaceID *string  `json:"workspace_id"`
	ProjectID   *string  `json:"project_id"`
	Name        string   `json:"name"`
	Labels      []string `json:"labels"`
	Size        *string  `json:"size"`
	Status      Status   `json:"status"`
	// Reason says why a job was refused, in words a customer can act on.
	Reason      string     `json:"reason"`
	Conclusion  string     `json:"conclusion"`
	RunnerName  string     `json:"runner_name"`
	QueuedAt    time.Time  `json:"queued_at"`
	StartedAt   *time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at"`
	// Seconds is how long it ran, once it has finished.
	Seconds *int `json:"seconds"`
}

// ByID names one job from the route.
type ByID struct {
	ID string `path:"runnerJobId" json:"-"`
}

const cols = `id::text, github_job_id::text, run_id::text, repository_id::text, repository, workspace_id::text, project_id::text,
  name, labels, size, status, reason, conclusion, runner_name, queued_at, started_at, completed_at,
  CASE WHEN started_at IS NOT NULL AND completed_at IS NOT NULL THEN greatest(0, extract(epoch FROM completed_at - started_at))::int END`

func scan(row pgx.Row) (Job, error) {
	var j Job
	err := row.Scan(&j.ID, &j.GitHubJobID, &j.RunID, &j.RepositoryID, &j.Repository, &j.WorkspaceID, &j.ProjectID,
		&j.Name, &j.Labels, &j.Size, &j.Status, &j.Reason, &j.Conclusion, &j.RunnerName, &j.QueuedAt, &j.StartedAt, &j.CompletedAt, &j.Seconds)
	return j, err
}

func one(c *mcpl.Call, id string) (Job, error) {
	j, err := scan(c.DB.QueryRow(c.Ctx, `SELECT `+cols+` FROM shippin_runner_jobs WHERE id::text=$1 AND tenant_id=$2`, id, c.Tenant))
	if errors.Is(err, pgx.ErrNoRows) {
		return j, mcpl.NotFound("No runner job " + id + " in this organization.")
	}
	return j, err
}
