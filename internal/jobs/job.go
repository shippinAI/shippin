// Package jobs is the durable job queue: rows in the jobs table, claimed with SKIP LOCKED under a
// renewable lease, so provisioning survives console restarts and runs once across replicas.
package jobs

import (
	"encoding/json"
	"errors"
	"time"
)

// Status values (the OpenAPI Job schema).
const (
	Queued    = "queued"
	Running   = "running"
	Succeeded = "succeeded"
	Failed    = "failed"
	Canceling = "canceling"
	Canceled  = "canceled"
)

// Kinds handled by the console.
const (
	KindWorkspaceAction     = "workspace.action"
	KindWorkspaceProvision  = "workspace.provision"
	KindTenantProvision     = "tenant.provision"
	KindBoundaryProvision   = "boundary.provision"
	KindAIActivate          = "ai.activate"          // M2: start a user's activation terminal (payload: optional logout)
	KindProjectImport       = "project.import"       // M3: create the first workspace of a GitHub-imported project
	KindProjectPrompt       = "project.from_prompt"  // M4: devspace for a prompt-built project, then its agent
	KindAgentStart          = "agent.start"          // M4: deliver the prompt and start the user's own CLI agent
	KindServiceProvision    = "service.provision"    // managed service instance (rrflow/state) + binding
	KindDeploymentReconcile = "deployment.reconcile" // desired deployment state; payload is identifier-only
)

var (
	ErrNotFound  = errors.New("job not found")
	ErrLostLease = errors.New("job lease lost")
)

// Job is one row of the jobs table.
type Job struct {
	LeaseOwner  string          `json:"-"`
	ID          string          `json:"id"`
	TenantID    string          `json:"tenant_id"`
	Kind        string          `json:"kind"`
	TargetType  string          `json:"target_type"`
	TargetID    string          `json:"target_id"`
	Status      string          `json:"status"`
	Step        string          `json:"step"`
	Attempts    int             `json:"attempts"`
	MaxAttempts int             `json:"max_attempts"`
	Payload     json.RawMessage `json:"payload,omitempty"`
	Result      json.RawMessage `json:"result,omitempty"`
	Error       string          `json:"error,omitempty"`
	CreatedBy   string          `json:"created_by"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	FinishedAt  *time.Time      `json:"finished_at,omitempty"`
}

// Done reports whether the job reached a terminal status.
func (j *Job) Done() bool {
	return j.Status == Succeeded || j.Status == Failed || j.Status == Canceled
}

// permanent marks a handler error that retrying can't fix.
type permanent struct{ err error }

func (p permanent) Error() string { return p.err.Error() }
func (p permanent) Unwrap() error { return p.err }

// Permanent wraps err so the runner fails the job without retrying.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanent{err}
}

// IsPermanent reports whether err (or anything it wraps) was marked Permanent.
func IsPermanent(err error) bool {
	var p permanent
	return errors.As(err, &p)
}
