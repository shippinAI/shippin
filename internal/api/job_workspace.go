package api

import (
	"context"
	"errors"
	"log"
	"net/http"

	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hostclient"
	"shippin.cloud/console/internal/jobs"
)

// acceptedWorkspace is the 202 body: the workspace plus the job provisioning it.
type acceptedWorkspace struct {
	workspaceView
	Job *jobOut `json:"job,omitempty"`
}

// startWorkspace hands a saved workspace to the job runner, or (jobs disabled) to a goroutine.
// An enqueue error isn't fatal: the reconciler re-enqueues workspaces left provisioning.
func (a *API) startWorkspace(w http.ResponseWriter, ctx context.Context, c *caller, t *tenantView, ws db.Workspace) {
	if a.runner == nil {
		go a.provisionWorkspace(*t, ws)
		writeStatus(w, http.StatusAccepted, a.view(t, ws, nil))
		return
	}
	j, _, err := a.jobs.Enqueue(ctx, a.workspaceJob(ws, c.id.Sub))
	if err != nil {
		log.Printf("workspace %s: enqueue: %v", ws.ID, err)
	} else {
		a.runner.Wake()
		w.Header().Set("Location", "/v1/jobs/"+j.ID)
	}
	writeStatus(w, http.StatusAccepted, acceptedWorkspace{a.view(t, ws, nil), jobView(j, c.op != "")})
}

func (a *API) workspaceJob(ws db.Workspace, by string) jobs.Job {
	return jobs.Job{TenantID: ws.TenantID, Kind: jobs.KindWorkspaceProvision, TargetType: "workspace",
		TargetID: ws.ID, MaxAttempts: a.cfg.Jobs.MaxAttempts, CreatedBy: by}
}

// runWorkspaceJob provisions one workspace. hostd's create is idempotent (existing volume and
// instance are reused), so a rerun after a crash continues where the last one stopped.
func (a *API) runWorkspaceJob(ctx context.Context, j *jobs.Job, step func(string)) (any, error) {
	ws, err := a.db.WorkspaceByID(ctx, j.TargetID)
	if err != nil {
		return nil, err
	}
	if ws == nil || ws.State != "provisioning" { // deleted, or finished by an earlier run
		return map[string]string{"skipped": "workspace is not provisioning"}, nil
	}
	tn, err := a.db.TenantByID(ctx, ws.TenantID)
	if err != nil || tn == nil {
		return nil, errors.Join(errors.New("tenant lookup failed"), err)
	}
	t := tenantView{Tenant: *tn}
	stop := a.trackProgress(t.ID, ws.ID, step)
	ip, err := a.host.CreateWorkspace(ctx, t.ID, ws.ID, a.createReq(ctx, &t, *ws))
	stop()
	if err != nil {
		if hostclient.Rejected(err) {
			err = jobs.Permanent(err)
		}
		if jobs.IsPermanent(err) || j.Attempts >= j.MaxAttempts {
			if e := a.jobs.FinishProvision(context.WithoutCancel(ctx), j, "failed", "", err.Error()); e != nil {
				return nil, e
			}
		}
		return nil, err
	}
	if err := a.jobs.FinishProvision(ctx, j, "running", ip, ""); err != nil {
		return nil, err
	}
	a.notifyWorkspaceReady(t.Name, ws)
	return map[string]string{"ip": ip}, nil
}
