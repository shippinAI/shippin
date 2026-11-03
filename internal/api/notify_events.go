package api

import (
	"context"
	"time"

	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/mail"
)

func grantVars(g db.Grant, actor string) map[string]string {
	return map[string]string{"Tenant": g.TenantName, "Admin": g.RequesterName, "Actor": actor}
}

// grantNotice reloads grant id (with tenant/workspace names and final times) and runs fn.
func (a *API) grantNotice(what, id string, fn func(ctx context.Context, g db.Grant) error) {
	a.notifyLater(what, func(ctx context.Context) error {
		g, err := a.db.GrantByID(ctx, id)
		if err != nil || g == nil {
			return err
		}
		return fn(ctx, *g)
	})
}

// notifyRequested tells the tenant's owners/admins about a new access request.
func (a *API) notifyRequested(id string) {
	a.grantNotice("access_request", id, func(ctx context.Context, g db.Grant) error {
		return a.noticeToManagers(ctx, "access_request", g, time.Time{}, grantVars(g, ""))
	})
}

// notifyBreakGlass tells the tenant's owners/admins, no earlier than the grant's notice time
// (later than now only for a legal-hold category, whose delay is itself audited).
func (a *API) notifyBreakGlass(id string) {
	a.grantNotice("breakglass", id, func(ctx context.Context, g db.Grant) error {
		return a.noticeToManagers(ctx, "breakglass", g, noticeTime(g), grantVars(g, ""))
	})
}

func noticeTime(g db.Grant) time.Time {
	if g.NoticeAt != nil {
		return *g.NoticeAt
	}
	return time.Time{}
}

var decisionKinds = map[string]string{"approve": "access_approved", "deny": "access_denied", "revoke": "access_revoked"}

// notifyDecided tells the requesting admin about an approve/deny/revoke.
func (a *API) notifyDecided(id, op, actor string) {
	kind, ok := decisionKinds[op]
	if !ok {
		return
	}
	a.grantNotice(kind, id, func(ctx context.Context, g db.Grant) error {
		return a.noticeToRequester(ctx, kind, g, grantVars(g, actor))
	})
}

// notifyExpired tells the requester and (after any notice delay) the tenant's managers.
func (a *API) notifyExpired(g db.Grant) {
	a.notifyLater("access_expired", func(ctx context.Context) error {
		if err := a.noticeToRequester(ctx, "access_expired", g, grantVars(g, "")); err != nil {
			return err
		}
		return a.noticeToManagers(ctx, "access_expired", g, noticeTime(g), grantVars(g, ""))
	})
}

// notifyWorkspaceReady tells the workspace's creator it is running; when the creator has no
// e-mail (an API/machine user) the tenant's managers are told instead.
func (a *API) notifyWorkspaceReady(tenant string, ws *db.Workspace) {
	a.notifyLater("workspace_ready", func(ctx context.Context) error {
		to, err := a.workspaceRecipients(ctx, ws)
		if err != nil || len(to) == 0 {
			return err
		}
		_, err = a.notify.Queue(ctx, mail.Notice{Kind: "workspace_ready", Vars: map[string]string{"Tenant": tenant, "Workspace": ws.Name},
			Facts: [][2]string{{"Workspace", ws.Name}, {"Address", ws.Host}}, URL: "https://" + ws.Host},
			to, ws.ID, time.Time{})
		return err
	})
}

func (a *API) workspaceRecipients(ctx context.Context, ws *db.Workspace) ([]string, error) {
	if to, err := a.emailOf(ctx, ws.CreatedBy); err != nil || to != "" {
		return []string{to}, err
	}
	org, err := a.tenantOrg(ctx, ws.TenantID)
	if err != nil || org == "" {
		return nil, err
	}
	return a.recipientsOf(ctx, org)
}
