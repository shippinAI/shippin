package api

import (
	"context"
	"errors"

	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/ext"
)

// lifecycle is the organization lifecycle the panel uses: the registered module's, or the current one.
func (a *API) lifecycle() ext.OrgLifecycle {
	if p := ext.Lifecycle(); p != nil {
		return p
	}
	return currentLifecycle{a}
}

// orgReady says whether work may start in t, and why not.
func (a *API) orgReady(t db.Tenant) (bool, string) {
	return a.lifecycle().Ready(ext.OrgState{ID: t.ID, Status: t.Status, Address: t.SSHIP})
}

// workReady is the readiness decision for starting work other than creating a workspace.
// A module's Ready is the whole answer, so a status comparison cannot bypass it. With no
// module, ready means status "active", or also "failed" when allowFailed (enabling a tenant
// VM). That is the check those call sites made on the status column. Creating a workspace
// keeps asking orgReady, which still waits for an address as well.
func (a *API) workReady(t db.Tenant, allowFailed bool) (ok bool, why string, module bool) {
	if ext.Lifecycle() != nil {
		ok, why = a.orgReady(t)
		return ok, why, true
	}
	if t.Status == "active" || (allowFailed && t.Status == "failed") {
		return true, "", false
	}
	return false, "", false
}

// notReadyReason is the message for a refused start. A module's own words are shown; with no
// module the call site keeps the message it had.
func notReadyReason(why string, module bool, fallback string) string {
	if !module {
		return fallback
	}
	if why == "" {
		return "This organization is not ready."
	}
	return why
}

// currentLifecycle is today's behaviour, until the per-customer isolation moves to the enterprise
// module (docs/editions.md): shippin-hostd builds the organization's silo, its workspaces are
// re-published on the new network, and its hosted Vaultix project is created.
type currentLifecycle struct{ a *API }

func (l currentLifecycle) Prepare(ctx context.Context, _ ext.Runtime, org ext.Org) (string, error) {
	t, err := l.a.db.TenantByID(ctx, org.ID)
	if err != nil {
		return "", err
	}
	if t == nil {
		return "", errors.New("organization not found")
	}
	p, err := l.a.host.Provision(ctx, t.ID, t.Plan)
	if err != nil {
		return "", err
	}
	t.Status, t.SSHIP = "active", p.SSHIP
	l.a.resyncWorkspaces(ctx, *t)
	go l.a.ensureVaultixWorkspace(t.ID, "")
	return p.SSHIP, nil
}

func (currentLifecycle) Ready(s ext.OrgState) (bool, string) {
	if s.Status != "active" || s.Address == "" {
		return false, "This tenant isn't provisioned yet."
	}
	return true, ""
}

func (l currentLifecycle) Remove(ctx context.Context, _ ext.Runtime, org ext.Org) (string, error) {
	return l.a.host.DeleteTenant(ctx, org.ID)
}
