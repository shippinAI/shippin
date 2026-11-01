package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hostd"
)

// createWorkspace inserts the row, returns 202 and provisions in the background.
func (a *API) createWorkspace(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.tenantCtx(w, r)
	if !ok {
		return
	}
	// Members only: a new workspace carries its creator's SSH keys, which would outlive a grant.
	if !c.canDecide(t) {
		auth.Problem(w, http.StatusForbidden, "Creating workspaces needs the org owner or admin role.")
		return
	}
	if t.Status != "active" || t.SSHIP == "" {
		auth.Problem(w, http.StatusConflict, "This tenant isn't provisioned yet.")
		return
	}
	a.idempotent(w, r, t.ID+":"+c.id.Sub, func(w http.ResponseWriter, r *http.Request) { a.newWorkspace(w, r, c, t) })
}

// newWorkspace validates the request, saves the row and starts provisioning.
func (a *API) newWorkspace(w http.ResponseWriter, r *http.Request, c *caller, t *tenantView) {
	var in struct {
		Name     string `json:"name"`
		Template string `json:"template_id"`
		Repo     string `json:"repo"`
		Size     string `json:"size"`
		Project  string `json:"project_id"`
		Env      string `json:"environment"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in) != nil {
		auth.Problem(w, http.StatusBadRequest, "Invalid request body.")
		return
	}
	if err := hostd.ValidLabel(in.Name); err != nil {
		auth.Problem(w, http.StatusBadRequest, "Name: "+err.Error())
		return
	}
	spec, msg := a.workspaceSpec(r.Context(), in.Template, strings.TrimSpace(in.Repo), in.Size)
	if msg != "" {
		auth.Problem(w, http.StatusBadRequest, msg)
		return
	}
	keys, err := a.userKeys(r, c.id.Sub)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "SSH keys are unavailable right now.")
		return
	}
	id, _ := uuid.NewV7()
	ws := &db.Workspace{ID: id.String(), TenantID: t.ID, Name: in.Name, State: "provisioning", SSHKeys: keys,
		CreatedBy: c.id.Sub, Host: a.wsHost(in.Name, id.String()),
		Template: spec.Template, Repo: spec.Repo, Size: spec.Size, Previews: spec.Previews}
	if !a.inProject(w, r, t.ID, in.Project, in.Env, ws) {
		return
	}
	if err := a.db.CreateWorkspace(r.Context(), ws, a.cfg.Workspaces.PortMin, a.cfg.Workspaces.PortMax); err != nil {
		switch {
		case errors.Is(err, db.ErrNameTaken):
			auth.Problem(w, http.StatusConflict, "A workspace with that name already exists.")
			return
		case errors.Is(err, db.ErrNoPorts):
			auth.Problem(w, http.StatusServiceUnavailable, "The host has no free SSH ports. Contact support.")
			return
		}
		auth.Problem(w, http.StatusServiceUnavailable, "Workspace could not be saved.")
		return
	}
	go a.ensureVaultixWorkspace(t.ID, ws.ID)
	if ws.ProjectID != "" && a.rrflowAutoAttach(r.Context(), t.ID) {
		if _, _, err := a.queueRRFlow(r.Context(), t.ID, ws.ProjectID, ws.ID, c.id.Sub); err != nil {
			log.Printf("workspace %s: rrflow auto-attach: %v", ws.ID, err) // optional: the devspace goes on without it
		}
	}
	a.startWorkspace(w, r.Context(), c, t, *ws)
}

func (a *API) provisionWorkspace(t tenantView, ws db.Workspace) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	stop := a.trackProgress(t.ID, ws.ID, nil)
	ip, err := a.host.CreateWorkspace(ctx, t.ID, ws.ID, a.createReq(ctx, &t, ws))
	stop()
	if err != nil {
		log.Printf("workspace %s failed: %v", ws.ID, err)
		_ = a.db.SetWorkspaceState(ctx, ws.ID, "failed", "", err.Error())
		return
	}
	if err := a.db.SetWorkspaceState(ctx, ws.ID, "running", ip, ""); err == nil {
		a.notifyWorkspaceReady(t.Name, &ws)
	}
}
