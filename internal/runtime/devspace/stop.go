package devspace

import "shippin.cloud/console/internal/mcpl"

// Stop is shippin.runtime.devspace.stop: POST /v1/tenants/{tenantId}/runtime/devspaces/{devspaceId}/stop.
var Stop = mcpl.Def[ByID, Accepted]{
	Spec: mcpl.Spec{Summary: "Power off a development space; its files are kept.", Access: mcpl.Member, Perm: "manage", Scoped: true, GrantPerm: "workspaces.act"},
	Run:  func(c *mcpl.Call, in ByID) (Accepted, error) { return apply(c, in.ID, "stop", "") },
}
