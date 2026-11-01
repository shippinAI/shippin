package api

import (
	"encoding/json"
	"net/http"
	"regexp"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hostclient"
)

// One-click Publish: the workspace app goes public at https://<slug>--site.<ws base> from a frozen
// release (hostd snapshots + copies the devspace), with Unpublish and Rollback. No sign-in is
// needed to view a published site, so friends and family just open the link.

// siteFolderRe mirrors hostd's gitPathRe: the app folder that is secret-scanned before a publish.
var siteFolderRe = regexp.MustCompile(`^/home/dev(/[A-Za-z0-9][A-Za-z0-9._-]{0,99}){1,6}$`)

var siteSlugRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$`)

func (a *API) siteRoutes(mux Router) {
	mux.HandleFunc("GET /v1/tenants/{tenantId}/workspaces/{workspaceId}/site", a.flow.Require(a.getSite))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/workspaces/{workspaceId}/site", a.flow.Require(a.postSite))
}

func (a *API) getSite(w http.ResponseWriter, r *http.Request) {
	_, t, ok := a.tenantCtx(w, r)
	if !ok {
		return
	}
	items, err := a.liveViews(r, t)
	ws := a.findWorkspace(r, items)
	if err != nil || ws == nil {
		auth.Problem(w, http.StatusNotFound, "Workspace not found.")
		return
	}
	if !a.cfg.Site.Enabled {
		writeJSON(w, map[string]any{"enabled": false, "published": false, "releases": []any{}, "note": "One-click Publish is switched off for this deployment."})
		return
	}
	st, err := a.host.Site(r.Context(), t.ID, ws.ID, r.URL.Query().Get("slug"))
	if err != nil {
		writeJSON(w, map[string]any{"published": false, "releases": []any{}, "note": "Start the workspace to see its site."})
		return
	}
	writeJSON(w, st)
}

func (a *API) postSite(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.tenantCtx(w, r)
	if !ok {
		return
	}
	items, err := a.liveViews(r, t)
	ws := a.findWorkspace(r, items)
	if err != nil || ws == nil {
		auth.Problem(w, http.StatusNotFound, "Workspace not found.")
		return
	}
	// Publishing is public exposure: org admins only (members holding a manager role; never a
	// grant holder or a plain workspace owner), and only while site.enabled is on.
	if !(t.member() && c.canManage(t)) {
		auth.Problem(w, http.StatusForbidden, "Only an org admin can publish.")
		return
	}
	var in hostclient.SiteReq
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil || !siteSlugRe.MatchString(in.Slug) {
		auth.Problem(w, http.StatusBadRequest, "Pick a site name: lowercase letters, digits and dashes.")
		return
	}
	if !a.cfg.Site.Enabled && in.Action != "unpublish" {
		auth.Problem(w, http.StatusForbidden, "One-click Publish is switched off for this deployment.")
		return
	}
	switch in.Action {
	case "publish", "rollback":
		if in.Action == "publish" && !siteFolderRe.MatchString(in.Path) {
			auth.Problem(w, http.StatusBadRequest, "Pick the app folder to publish (under /home/dev).")
			return
		}
		if in.Port < 1024 || in.Port > 65535 {
			auth.Problem(w, http.StatusBadRequest, "Pick the port your app listens on.")
			return
		}
	case "unpublish":
	default:
		auth.Problem(w, http.StatusBadRequest, "Action must be publish, unpublish or rollback.")
		return
	}
	if !a.auditUse(r, c, t, ws.ID, "change", "site."+in.Action) {
		refuseUnaudited(w)
		return
	}
	if in.Action != "unpublish" && ws.State != "running" {
		auth.Problem(w, http.StatusConflict, "Start the workspace to publish it.")
		return
	}
	st, err := a.host.SiteAction(r.Context(), t.ID, ws.ID, in)
	if err != nil {
		auth.Problem(w, http.StatusConflict, "Couldn't "+in.Action+": "+err.Error())
		return
	}
	act := "site." + in.Action
	if in.DryRun {
		act += ".dry_run"
	}
	_ = a.store().AddAudit(r.Context(), db.AuditEntry{TenantID: t.ID, TenantName: t.Name, WorkspaceID: ws.ID,
		Actor: c.id.Sub, ActorName: c.id.Name, Action: act, Detail: in.Slug + " " + st.Current, IP: clientIP(r)})
	writeJSON(w, st)
}
