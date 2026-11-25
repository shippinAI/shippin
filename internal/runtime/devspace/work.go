package devspace

import (
	"context"
	"errors"
	"time"

	"shippin.cloud/console/internal/mcpl"
)

// States are the state a space is put in when a lifecycle action is accepted, and the one the host
// must report for the action to have worked.
var States = map[string][2]string{
	"start": {"starting", "running"}, "stop": {"stopping", "stopped"},
	"restart": {"starting", "running"}, "delete": {"deleting", "deleted"},
}

// Work is the durable record of one accepted lifecycle action: what the job that carries it out
// reads. It holds the grant the person acted under, never a token or a credential, so the grant is
// checked again when the work runs.
type Work struct {
	Action     string `json:"action"`
	Grant      string `json:"grant,omitempty"`
	Phase      string `json:"phase,omitempty"`
	Completion *struct {
		State   string `json:"state"`
		Message string `json:"message"`
	} `json:"completion,omitempty"`
}

// Queue is the panel's durable work queue, as a development space uses it.
type Queue interface {
	// Apply claims w and records work on it, on q, the action's own transaction: the claim, the work,
	// its audit entry and the answer a repeated request gets are kept or lost together. ErrBusy when
	// the space already has work in progress.
	Apply(ctx context.Context, q mcpl.DB, w Devspace, work Work, by string) (Job, error)
	// Wake tells the workers there is work, once it is committed.
	Wake()
}

// ErrBusy is Queue.Apply on a space with work in progress.
var ErrBusy = errors.New("the development space has work in progress")

// Job is the work a lifecycle action was accepted as. Its progress is read at /v1/jobs/{id}.
type Job struct {
	ID         string     `json:"id"`
	TenantID   string     `json:"tenant_id"`
	Kind       string     `json:"kind"`
	Target     JobTarget  `json:"target"`
	Status     string     `json:"status"`
	Message    string     `json:"message,omitempty"`
	Attempts   int        `json:"attempts"`
	CreatedBy  string     `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// JobTarget is what a job works on.
type JobTarget struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// Accepted is the answer to a lifecycle action: the space, the state it is now in, and the job doing
// the work. It is answered 202 with Location set to the job.
type Accepted struct {
	ID    string `json:"id"`
	State string `json:"state"`
	Job   Job    `json:"job"`
}

// AcceptedAt is where the work's progress is read.
func (a Accepted) AcceptedAt() string {
	if a.Job.ID == "" {
		return ""
	}
	return "/v1/jobs/" + a.Job.ID
}

// apply accepts one lifecycle action on the space the route names. The gate has already asked
// whether the caller manages this space; a person acting under a support grant deletes only by
// typing the space's name.
func apply(c *mcpl.Call, id, action, confirm string) (Accepted, error) {
	queue, err := mcpl.Need[Queue](c)
	if err != nil {
		return Accepted{}, err
	}
	orgs, err := mcpl.Need[Orgs](c)
	if err != nil {
		return Accepted{}, err
	}
	org, err := orgs.Org(c.Ctx, c.Tenant)
	if err != nil {
		return Accepted{}, err
	}
	w, err := On(c.Pool).ActionTarget(c.Ctx, id)
	if err != nil {
		return Accepted{}, err
	}
	if org == nil || w == nil || w.TenantID != c.Tenant {
		return Accepted{}, mcpl.NotFound("Development space not found.")
	}
	if action == "delete" && c.Grant != "" && confirm != w.Name {
		return Accepted{}, mcpl.Invalid("Type the development space's name exactly to confirm deletion.")
	}
	if org.Status == "suspended" && (action == "start" || action == "restart") {
		return Accepted{}, mcpl.Conflict("This organization is suspended. Resume it before starting development spaces.")
	}
	if org.Status == "deleting" || org.Status == "deleted" {
		return Accepted{}, mcpl.Conflict("This organization is being deleted.")
	}
	job, err := queue.Apply(c.Ctx, c.DB, *w, Work{Action: action, Grant: c.Grant}, c.Actor)
	if errors.Is(err, ErrBusy) {
		return Accepted{}, mcpl.Conflict("Wait until the current operation finishes.")
	}
	if err != nil {
		return Accepted{}, err
	}
	return Accepted{ID: w.ID, State: States[action][0], Job: job}, nil
}
