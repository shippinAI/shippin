// Package runner is the single-use machines that run organizations' build jobs (RUN-01,
// docs/runners.md): one virtual machine per runner job, started while the organization is under its
// plan's limit and removed when the job ends or outlives the job time limit. The panel starts and
// removes them through durable jobs (dispatch.go); staff list every machine and remove a stuck one.
// Organizations see their jobs through runtime/runner_job, never the machines.
package runner

import (
	"time"
)

// State is how far a machine has got.
type State string

// Values are the states: being made, running its job, being removed, gone, or failed (to start or to
// be removed; removal is retried).
func (State) Values() []string {
	return []string{"starting", "running", "removing", "removed", "failed"}
}

const (
	Starting State = "starting"
	Running  State = "running"
	Removing State = "removing"
	Removed  State = "removed"
	Failed   State = "failed"
)

// Runner is one runner machine as staff read it.
type Runner struct {
	ID           string `json:"id"`
	TenantID     string `json:"tenant_id"`
	Organization string `json:"organization"`
	RunnerJobID  string `json:"runner_job_id"`
	GitHubJobID  string `json:"github_job_id"`
	Repository   string `json:"repository"`
	// AssignedJobID is the job GitHub gave this machine's runner, once it says so; GitHub may give a
	// waiting runner any queued job of the same repository and labels.
	AssignedJobID *string `json:"assigned_job_id"`
	ServerID      *string `json:"server_id"`
	Server        string  `json:"server"`
	Name          string  `json:"name"`
	// Guest is the hypervisor's id for the machine, which is also its runner's name on GitHub.
	Guest   string `json:"guest"`
	Size    string `json:"size"`
	State   State  `json:"state"`
	Attempt int    `json:"attempt"`
	// Error is what last went wrong starting or removing it.
	Error         string     `json:"error"`
	RemovalReason string     `json:"removal_reason"`
	CreatedAt     time.Time  `json:"created_at"`
	StartedAt     *time.Time `json:"started_at"`
	RemovedAt     *time.Time `json:"removed_at"`
	// AgeSeconds is how long it has existed, or existed for once removed.
	AgeSeconds int `json:"age_seconds"`
}

// ByID names one machine from the route.
type ByID struct {
	ID string `path:"runnerId" json:"-"`
}

const cols = `m.id::text, m.tenant_id::text, t.name, m.runner_job_id::text, j.github_job_id::text, j.repository, m.assigned_job_id::text,
  m.server_id::text, coalesce(s.slug, ''), m.name, m.guest, m.size, m.state, m.attempt, m.error, m.removal_reason, m.created_at, m.started_at, m.removed_at,
  greatest(0, extract(epoch FROM coalesce(m.removed_at, now()) - m.created_at))::int`

const from = ` FROM shippin_runner_machines m JOIN tenants t ON t.id = m.tenant_id
  JOIN shippin_runner_jobs j ON j.id = m.runner_job_id AND j.tenant_id = m.tenant_id
  LEFT JOIN servers s ON s.id = m.server_id`

type row interface{ Scan(...any) error }

func scan(r row) (Runner, error) {
	var m Runner
	err := r.Scan(&m.ID, &m.TenantID, &m.Organization, &m.RunnerJobID, &m.GitHubJobID, &m.Repository, &m.AssignedJobID,
		&m.ServerID, &m.Server, &m.Name, &m.Guest, &m.Size, &m.State, &m.Attempt, &m.Error, &m.RemovalReason, &m.CreatedAt, &m.StartedAt, &m.RemovedAt,
		&m.AgeSeconds)
	return m, err
}
