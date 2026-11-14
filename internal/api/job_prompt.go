package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/agentruns"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/jobs"
	"shippin.cloud/console/internal/projects"
)

// runPromptJob (project.from_prompt, implementation-plan-3 M4 "Job steps"): project placeable →
// devspace from the guessed template (provisioned inline, -ai1 image with the user's AI home) →
// rrflow attach (optional) → the agent step (deliverAndStart).
func (a *API) runPromptJob(ctx context.Context, j *jobs.Job, step func(string)) (any, error) {
	var in struct {
		RunID string `json:"run_id"`
	}
	if json.Unmarshal(j.Payload, &in) != nil || in.RunID == "" {
		return nil, jobs.Permanent(errors.New("bad project.from_prompt payload"))
	}
	run, err := a.runs.Get(ctx, j.TenantID, in.RunID)
	if err != nil {
		return nil, jobs.Permanent(err)
	}
	if run.State == agentruns.Running || run.State == agentruns.Exited || run.State == agentruns.WaitingAI {
		return map[string]string{"skipped": "agent already " + run.State}, nil
	}
	name, err := a.db.ProjectName(ctx, j.TenantID, j.TargetID)
	if err != nil || name == "" {
		return nil, jobs.Permanent(errors.New("project not found"))
	}
	step("Waiting for your private server")
	pl, err := (&projects.Store{Pool: a.db.Pool}).Place(ctx, j.TenantID, j.TargetID, "dev")
	if err != nil {
		return nil, errors.New("project is not ready yet")
	}
	ws, err := a.promptWorkspace(ctx, j, run, name, pl)
	if err != nil {
		return nil, err
	}
	_ = a.runs.SetWorkspace(ctx, run.ID, ws.ID)
	if ws.State == "provisioning" {
		_ = a.runs.SetState(ctx, run.ID, agentruns.Provisioning, "")
		wj := a.workspaceJob(*ws, j.CreatedBy)
		wj.Attempts, wj.MaxAttempts = j.Attempts, j.MaxAttempts
		if _, err := a.runWorkspaceJob(ctx, &wj, step); err != nil {
			if jobs.IsPermanent(err) || j.Attempts >= j.MaxAttempts {
				_ = a.runs.SetState(context.WithoutCancel(ctx), run.ID, agentruns.Failed, "Your devspace could not be created.")
			}
			return nil, err
		}
	}
	if ws, err = a.db.WorkspaceByID(ctx, ws.ID); err != nil || ws == nil || ws.State != "running" {
		return nil, errors.New("devspace is not running yet")
	}
	if (a.cfg.Prompt.AttachRRFlow && a.cfg.Services.RRFlow.Allows(j.TenantID)) || a.rrflowAutoAttach(ctx, j.TenantID) {
		step("Attaching rrflow")
		if err := a.attachRRFlow(ctx, j.TenantID, j.TargetID, ws.ID, j.CreatedBy); err != nil {
			log.Printf("prompt %s: rrflow attach: %v", j.TargetID, err) // optional: the build goes on without it
		}
	}
	return a.deliverAndStart(ctx, run, ws.ID, false, step)
}

// promptWorkspace returns the project's devspace, creating its row (provisioning) on first run.
func (a *API) promptWorkspace(ctx context.Context, j *jobs.Job, run *agentruns.Run, name string, pl projects.Placement) (*db.Workspace, error) {
	if all, err := a.db.Workspaces(ctx, j.TenantID); err == nil {
		for _, w := range all {
			if w.ProjectID == j.TargetID && w.State != "deleted" && w.State != "failed" {
				return &w, nil
			}
		}
	}
	spec, msg := a.workspaceSpec(ctx, run.Template, "", "")
	if msg != "" {
		return nil, jobs.Permanent(errors.New(msg))
	}
	pub, _ := a.userKeys(ctx, j.CreatedBy)
	id, _ := uuid.NewV7()
	label := strings.TrimRight(name[:min(len(name), 31)], "-")
	ws := db.Workspace{ID: id.String(), TenantID: j.TenantID, Name: label, State: "provisioning", SSHKeys: pub,
		CreatedBy: j.CreatedBy, Host: a.wsHost(label, id.String()), Template: spec.Template, Size: spec.Size,
		Previews: spec.Previews, ProjectID: j.TargetID, EnvironmentID: pl.EnvID}
	if pl.Boundary != nil {
		ws.BoundaryID = *pl.Boundary
	}
	if err := a.db.CreateWorkspace(ctx, &ws, a.cfg.Workspaces.PortMin, a.cfg.Workspaces.PortMax); err != nil {
		return nil, err
	}
	return &ws, nil
}

// promptDetached runs project.from_prompt without the job runner, retrying while things come up.
func (a *API) promptDetached(j jobs.Job) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	for i := 1; i <= 30; i++ {
		j.Attempts, j.MaxAttempts = i, 30
		_, err := a.runPromptJob(ctx, &j, func(string) {})
		if err == nil || ctx.Err() != nil || jobs.IsPermanent(err) {
			if err != nil {
				log.Printf("prompt %s: %v", j.TargetID, err)
			}
			return
		}
		time.Sleep(time.Minute)
	}
}
