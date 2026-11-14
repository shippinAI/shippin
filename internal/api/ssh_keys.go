package api

import (
	"context"

	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/identity/ssh_key"
	"shippin.cloud/console/internal/mcpl"
)

// SSH keys are actions in internal/identity/ssh_key. When a person adds or removes one, their running
// workspaces get the new set.
func (a *API) followSSHKeys() {
	push := func(ctx context.Context, e mcpl.Event) { a.pushKeys(ctx, e.Actor) }
	a.events.On("shippin.identity.ssh_key.created", push)
	a.events.On("shippin.identity.ssh_key.deleted", push)
}

func (a *API) userKeys(ctx context.Context, sub string) ([]string, error) {
	return ssh_key.PublicKeys(ctx, a.db.Pool, sub)
}

// pushKeys applies the user's current keys to every running workspace they created.
func (a *API) pushKeys(ctx context.Context, sub string) {
	keys, err := a.userKeys(ctx, sub)
	if err != nil {
		return
	}
	wss, _ := a.db.WorkspacesByCreator(ctx, sub)
	for _, ws := range wss {
		_ = a.db.SetWorkspaceKeys(ctx, ws.ID, keys)
		if ws.State == "running" {
			ws.SSHKeys = keys
			t := tenantView{Tenant: db.Tenant{ID: ws.TenantID}}
			_ = a.host.Action(ctx, ws.TenantID, ws.ID, "keys", a.hostReq(&t, ws))
		}
	}
}
