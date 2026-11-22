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
