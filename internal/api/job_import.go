package api

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/jobs"
	"shippin.cloud/console/internal/projects"
)

// runImportJob (project.import) waits for the project to be placeable, then creates its first
// workspace cloning github:<repo>@<branch> and hands it to workspace provisioning.
func (a *API) runImportJob(ctx context.Context, j *jobs.Job, step func(string)) (any, error) {
	src, err := a.db.SourceOf(ctx, j.TargetID)
	if err != nil {
		return nil, err
	}
	if src == nil || src.Kind != "github" {
		return nil, jobs.Permanent(errors.New("project has no GitHub source"))
	}
	if src.Status != "ok" {
		return nil, jobs.Permanent(errors.New("repository access was revoked"))
	}
	name, err := a.db.ProjectName(ctx, j.TenantID, j.TargetID)
	if err != nil || name == "" {
		return nil, jobs.Permanent(errors.New("project not found"))
	}
	if ws, _ := a.db.Workspaces(ctx, j.TenantID); hasProjectWorkspace(ws, j.TargetID) {
		return map[string]string{"skipped": "project already has a workspace"}, nil
	}
	step("Waiting for the project")
	p, err := (&projects.Store{Pool: a.db.Pool}).Place(ctx, j.TenantID, j.TargetID, "dev")
	if err != nil {
		return nil, errors.New("project is not ready yet")
	}
	pub, _ := a.userKeys(ctx, j.CreatedBy)
	id, _ := uuid.NewV7()
	label := strings.TrimRight(name[:min(len(name), 31)], "-")
	ws := db.Workspace{ID: id.String(), TenantID: j.TenantID, Name: label, State: "provisioning", SSHKeys: pub,
		CreatedBy: j.CreatedBy, Host: a.wsHost(label, id.String()), Repo: "github:" + src.RepoFullName + "@" + src.Branch,
		ProjectID: j.TargetID, EnvironmentID: p.EnvID}
	if p.Boundary != nil {
		ws.BoundaryID = *p.Boundary
	}
	step("Creating the workspace")
	if err := a.db.CreateWorkspace(ctx, &ws, a.cfg.Workspaces.PortMin, a.cfg.Workspaces.PortMax); err != nil {
		return nil, err
	}
	if a.runner == nil {
		tn, err := a.db.TenantByID(ctx, j.TenantID)
		if err != nil || tn == nil {
			return nil, errors.New("tenant lookup failed")
		}
		a.provisionWorkspace(tenantView{Tenant: *tn}, ws)
	} else {
		a.enqueue(ctx, a.workspaceJob(ws, j.CreatedBy))
	}
	return map[string]string{"workspace_id": ws.ID}, nil
}

func hasProjectWorkspace(ws []db.Workspace, project string) bool {
	for _, w := range ws {
		if w.ProjectID == project && w.State != "deleted" && w.State != "failed" {
			return true
		}
	}
	return false
}

// importDetached runs project.import without the job runner (jobs disabled), retrying while the
// project's boundary comes up.
func (a *API) importDetached(j jobs.Job) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	for i := 0; i < 30; i++ {
		_, err := a.runImportJob(ctx, &j, func(string) {})
		if err == nil || ctx.Err() != nil || jobs.IsPermanent(err) {
			if err != nil {
				log.Printf("import %s: %v", j.TargetID, err)
			}
			return
		}
		time.Sleep(time.Minute)
	}
}
