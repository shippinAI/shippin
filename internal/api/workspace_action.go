package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/jobs"
)

var actionState = map[string][2]string{
	"start": {"starting", "running"}, "stop": {"stopping", "stopped"},
	"restart": {"starting", "running"}, "delete": {"deleting", "deleted"},
}

// workspaceAction runs start/stop/restart/delete in the background (202 + polling).
func (a *API) workspaceAction(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.tenantCtx(w, r)
	if !ok {
		return
	}
	act := r.PathValue("action")
	if _, known := actionState[act]; !known {
		auth.Problem(w, http.StatusBadRequest, "Unknown action.")
		return
	}
	ws, err := a.db.WorkspaceActionTarget(r.Context(), r.PathValue("workspaceId"))
	if err != nil || ws == nil || ws.TenantID != t.ID {
		auth.Problem(w, http.StatusNotFound, "Workspace not found.")
		return
	}
	if !c.canManageWS(t, ws.ID) {
		auth.Problem(w, http.StatusForbidden, "This needs the org owner or admin role.")
		return
	}
	if !a.auditUse(r, c, t, ws.ID, "action", act) {
		refuseUnaudited(w)
		return
	}
	a.runAction(w, r, t, *ws, act, c.id.Sub)
}

// runAction is the one path for lifecycle actions (owner and provider panel): it checks the
// tenant state, claims the workspace atomically (no two operations at once), then runs hostd.
func (a *API) runAction(w http.ResponseWriter, r *http.Request, t *tenantView, ws db.Workspace, act, who string) {
	a.idempotent(w, r, t.ID+":"+strings.TrimPrefix(who, "operator "), func(w http.ResponseWriter, r *http.Request) {
		a.enqueueWorkspaceAction(w, r, t, ws, act, who)
	})
}

func (a *API) enqueueWorkspaceAction(w http.ResponseWriter, r *http.Request, t *tenantView, ws db.Workspace, act, who string) {
	st := actionState[act]
	if t.Status == "suspended" && (act == "start" || act == "restart") {
		auth.Problem(w, http.StatusConflict, "This tenant is suspended. Resume it before starting workspaces.")
		return
	}
	if t.Status == "deleting" || t.Status == "deleted" {
		auth.Problem(w, http.StatusConflict, "This tenant is being deleted.")
		return
	}
	if a.runner == nil {
		auth.Problem(w, 503, "Durable operations are unavailable. No action was started.")
		return
	}
	in := workspaceActionIn{Action: act}
	if g := t.grantFor(ws.ID); g != nil {
		in.Grant = g.ID
	}
	payload, _ := json.Marshal(in)
	j, err := a.jobs.EnqueueAction(r.Context(), jobs.Job{TenantID: t.ID, TargetID: ws.ID, CreatedBy: strings.TrimPrefix(who, "operator "), MaxAttempts: max(a.cfg.Jobs.MaxAttempts, 1), Payload: payload}, st[0], jobs.ActionAcceptance{
		Scope: t.ID + ":" + strings.TrimPrefix(who, "operator "), Key: r.Header.Get("Idempotency-Key"),
		Response: func(j *jobs.Job) ([]byte, error) {
			return json.Marshal(map[string]any{"id": ws.ID, "state": st[0], "job": jobView(j, false)})
		},
	})
	if errors.Is(err, jobs.ErrWorkspaceBusy) {
		auth.Problem(w, 409, "Wait until the current operation finishes.")
		return
	}
	if err != nil {
		auth.Problem(w, 503, "The operation could not be recorded. No action was started.")
		return
	}
	a.runner.Wake()
	w.Header().Set("Location", "/v1/jobs/"+j.ID)
	writeStatus(w, http.StatusAccepted, map[string]any{"id": ws.ID, "state": st[0], "job": jobView(j, false)})
}
