package devspace

import "shippin.cloud/console/internal/mcpl"

// Restart is shippin.runtime.devspace.restart: POST /v1/tenants/{tenantId}/runtime/devspaces/{devspaceId}/restart.
var Restart = mcpl.Def[ByID, Accepted]{
	Spec: mcpl.Spec{Summary: "Restart a running development space.", Access: mcpl.Member, Perm: "manage", Scoped: true, GrantPerm: "workspaces.act"},
	Run:  func(c *mcpl.Call, in ByID) (Accepted, error) { return apply(c, in.ID, "restart", "") },
}
