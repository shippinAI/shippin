package devspace

import "shippin.cloud/console/internal/mcpl"

// Start is shippin.runtime.devspace.start: POST /v1/tenants/{tenantId}/runtime/devspaces/{devspaceId}/start.
var Start = mcpl.Def[ByID, Accepted]{
	Spec: mcpl.Spec{Summary: "Power on a development space.", Access: mcpl.Member, Perm: "manage", Scoped: true, GrantPerm: "workspaces.act"},
	Run:  func(c *mcpl.Call, in ByID) (Accepted, error) { return apply(c, in.ID, "start", "") },
}
