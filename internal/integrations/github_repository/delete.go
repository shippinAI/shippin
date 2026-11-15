package github_repository

import "shippin.cloud/console/internal/mcpl"

// Delete is shippin.integrations.github_repository.delete: DELETE /v1/tenants/{tenantId}/integrations/github-repositories/{githubRepositoryId}.
// The repository stays on GitHub and in the list; its new jobs no longer get Shippin runners, and its
// job history stays.
var Delete = mcpl.Def[ByID, mcpl.None]{
	Spec: mcpl.Spec{Summary: "Unassign a GitHub repository, so its jobs no longer get Shippin runners.", Access: mcpl.Member, Perm: "manage"},
	Run: func(c *mcpl.Call, in ByID) (mcpl.None, error) {
		id, err := in.number()
		if err != nil {
			return mcpl.None{}, err
		}
		tag, err := c.DB.Exec(c.Ctx, `DELETE FROM shippin_github_repositories WHERE id=$1 AND tenant_id=$2`, id, c.Tenant)
		if err == nil && tag.RowsAffected() == 0 {
			return mcpl.None{}, mcpl.NotFound("Repository " + in.ID + " is not assigned in this organization.")
		}
		return mcpl.None{}, err
	},
}
