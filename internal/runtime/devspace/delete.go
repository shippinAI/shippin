package devspace

import "shippin.cloud/console/internal/mcpl"

// DeleteInput names the space and, for a person acting under a support grant, repeats its name.
type DeleteInput struct {
	ByID
	// Confirm is the space's name, typed. A person acting under a support grant must send it.
	Confirm string `json:"confirm,omitempty"`
}

// Delete is shippin.runtime.devspace.delete: DELETE /v1/tenants/{tenantId}/runtime/devspaces/{devspaceId}.
// The space, its home volume and its address are removed by the job the answer names.
var Delete = mcpl.Def[DeleteInput, Accepted]{
	Spec: mcpl.Spec{Summary: "Delete a development space with its files and its address; this cannot be undone.", Access: mcpl.Member, Perm: "manage", Scoped: true, GrantPerm: "workspaces.act"},
	Run:  func(c *mcpl.Call, in DeleteInput) (Accepted, error) { return apply(c, in.ID, "delete", in.Confirm) },
}
