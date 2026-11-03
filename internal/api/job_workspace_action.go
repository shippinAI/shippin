package api

import (
	"context"
	"encoding/json"
	"errors"
	"shippin.cloud/console/internal/jobs"
	"strings"
	"time"
)

type workspaceActionIn struct {
	Action     string `json:"action"`
	Grant      string `json:"grant,omitempty"`
	Phase      string `json:"phase,omitempty"`
	Completion *struct {
		State   string `json:"state"`
		Message string `json:"message"`
	} `json:"completion,omitempty"`
}

// Accepted member authority is captured by the durable intent. Delegated access is rechecked
// before dispatch; tokens and credentials are never retained in a job.
func (a *API) runWorkspaceActionJob(ctx context.Context, j *jobs.Job, step func(string)) (any, error) {
	var in workspaceActionIn
	if json.Unmarshal(j.Payload, &in) != nil {
		return nil, jobs.Permanent(errors.New("invalid operation"))
	}
	states, ok := actionState[in.Action]
	if !ok {
		return nil, jobs.Permanent(errors.New("unknown operation"))
	}
	if in.Completion != nil {
		if in.Completion.State == "failed" {
			return nil, jobs.Permanent(errors.New(in.Completion.Message))
		}
		return map[string]string{"state": in.Completion.State}, nil
	}
	finish := func(state, msg string) (any, error) {
		if err := a.jobs.FinishAction(ctx, j, state, msg); err != nil {
			return nil, err
		}
		if msg != "" {
			return nil, jobs.Permanent(errors.New(msg))
		}
		return map[string]string{"state": state}, nil
	}
	ws, err := a.db.WorkspaceByID(ctx, j.TargetID)
	if err != nil {
		return nil, err
	}
	if ws == nil || ws.TenantID != j.TenantID {
		return nil, jobs.Permanent(errors.New("workspace not found"))
	}
	tn, err := a.db.TenantByID(ctx, j.TenantID)
	if err != nil {
		return nil, err
	}
	if tn == nil {
		return finish("failed", "Tenant not found.")
	}
	t := tenantView{Tenant: *tn}
	if in.Phase == "" {
		if tn.Status == "deleted" || tn.Status == "deleting" || tn.Status == "suspended" && (in.Action == "start" || in.Action == "restart") {
			return finish("failed", "Tenant state prevents this operation.")
		}
		if in.Grant != "" {
			g, err := a.db.GrantByID(ctx, in.Grant)
			if err != nil {
				return nil, err
			}
			if g == nil || g.Requester != j.CreatedBy || g.TenantID != j.TenantID || !g.Covers(ws.ID) || !g.Active(time.Now()) {
				return finish("failed", "The authorizing access grant is no longer active.")
			}
		}
		if err := a.jobs.ActionPhase(ctx, j, "dispatched"); err != nil {
			return nil, err
		}
		step("Applying " + in.Action)
		if err := a.host.Action(ctx, j.TenantID, ws.ID, in.Action, a.hostReq(&t, *ws)); err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return finish("failed", "Operation outcome is uncertain. Refresh inventory and review before retrying.")
		}
		if err := a.jobs.ActionPhase(ctx, j, "acknowledged"); err != nil {
			return nil, err
		}
		in.Phase = "acknowledged"
	}
	if in.Phase == "dispatched" && (in.Action == "restart" || in.Action == "delete") {
		return finish("failed", "Operation outcome is uncertain after interruption; it was not repeated.")
	}
	step("Verifying workspace state")
	inv, partial, err := a.host.TenantInventory(ctx, j.TenantID)
	if err != nil || partial {
		return finish("failed", "Operation completed or was dispatched, but inventory is unavailable. Outcome requires review.")
	}
	found, observed := false, ""
	for _, i := range inv {
		if i.Name == "w-"+ws.ID {
			found = true
			observed = strings.ToLower(i.Status)
		}
	}
	if in.Action == "delete" && !found && in.Phase == "acknowledged" {
		return finish("deleted", "")
	}
	if found && observed == states[1] {
		// Preserve the existing refresh after boot: workspace /run is temporary.
		a.pushTokenAfterStart(ctx, j.TenantID, *ws, in.Action)
		return finish(states[1], "")
	}
	return finish("failed", "Observed workspace state does not confirm the requested operation. It was not repeated.")
}
