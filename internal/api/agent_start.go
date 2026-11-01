package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	"shippin.cloud/console/internal/agentruns"
	"shippin.cloud/console/internal/jobs"
	"shippin.cloud/console/internal/subs"
)

// deliverAndStart is the agent step: the run's owner must be signed in to the provider with their
// own subscription (M2 status "connected"), else the run parks in waiting_ai until they connect.
// A provider login is not a workspace execution grant. The legacy shared-volume
// runner is blocked until the central Devagotchi owner-runner adapter is available.
func (a *API) deliverAndStart(ctx context.Context, run *agentruns.Run, ws string, resume bool, step func(string)) (any, error) {
	label := run.Provider
	if p, ok := a.cfg.AI.Provider(run.Provider); ok {
		label = p.Label
	}
	step("Checking your " + label + " sign-in")
	if !a.agentConnected(ctx, run) {
		_ = a.runs.SetState(ctx, run.ID, agentruns.WaitingAI, "")
		return map[string]string{"state": agentruns.WaitingAI}, nil
	}
	err := errors.New("Your private runner is connected, but the central Devagotchi workspace connection is not available yet. Your prompt is preserved.")
	_ = a.runs.SetState(ctx, run.ID, agentruns.Failed, err.Error())
	return nil, jobs.Permanent(err)
}

// agentConnected: the run owner's AI home in this tenant reports them signed in to the provider.
func (a *API) agentConnected(ctx context.Context, run *agentruns.Run) bool {
	h, err := a.subs.HomeFor(ctx, run.TenantID, run.AccountID, "")
	if err != nil || h.State != "ready" {
		return false
	}
	conns, err := a.subs.Connections(ctx, run.AccountID)
	if err != nil {
		return false
	}
	for _, c := range conns {
		if c.HomeID == h.ID && c.ProviderID == run.Provider && c.Status == subs.Connected {
			return true
		}
	}
	return false
}

type agentStartIn struct {
	Resume bool `json:"resume"`
}

// queueAgentStart enqueues agent.start for a run (or runs it in the background, jobs disabled).
func (a *API) queueAgentStart(ctx context.Context, run *agentruns.Run, by string, resume bool) *jobs.Job {
	payload, _ := json.Marshal(agentStartIn{Resume: resume})
	j := jobs.Job{TenantID: run.TenantID, Kind: jobs.KindAgentStart, TargetType: "agent_run", TargetID: run.ID,
		MaxAttempts: a.cfg.Jobs.MaxAttempts, CreatedBy: by, Payload: payload}
	if a.runner == nil {
		go func() {
			if _, err := a.runAgentStartJob(context.Background(), &j, func(string) {}); err != nil {
				log.Printf("agent run %s: %v", run.ID, err)
			}
		}()
		return nil
	}
	got, _, err := a.jobs.Enqueue(ctx, j)
	if err != nil {
		log.Printf("agent run %s: enqueue: %v", run.ID, err)
		return nil
	}
	a.runner.Wake()
	return got
}

// runAgentStartJob (agent.start) re-runs the agent step for a run whose devspace exists.
func (a *API) runAgentStartJob(ctx context.Context, j *jobs.Job, step func(string)) (any, error) {
	var in agentStartIn
	_ = json.Unmarshal(j.Payload, &in)
	run, err := a.runs.Get(ctx, j.TenantID, j.TargetID)
	if err != nil {
		return nil, jobs.Permanent(err)
	}
	if run.WorkspaceID == "" {
		return nil, jobs.Permanent(errors.New("the build's devspace isn't ready yet"))
	}
	ws, err := a.db.WorkspaceByID(ctx, run.WorkspaceID)
	if err != nil || ws == nil || ws.State != "running" {
		return nil, errors.New("devspace is not running")
	}
	return a.deliverAndStart(ctx, run, ws.ID, in.Resume, step)
}

// resumeWaiting starts every run of this AI home's owner that was parked on a sign-in which is
// now connected (called after a status check).
func (a *API) resumeWaiting(ctx context.Context, h *subs.Home) {
	if a.runs == nil {
		return
	}
	runs, err := a.runs.Waiting(ctx, h.AccountID, h.TenantID)
	if err != nil {
		return
	}
	for i := range runs {
		if a.agentConnected(ctx, &runs[i]) {
			a.queueAgentStart(ctx, &runs[i], runs[i].CreatedBy, false)
		}
	}
}
