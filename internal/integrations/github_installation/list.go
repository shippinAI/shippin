package github_installation

import "shippin.cloud/console/internal/mcpl"

// List is shippin.integrations.github_installation.list: GET /v1/tenants/{tenantId}/integrations/github-installations.
// It reads only what the panel recorded, so it answers even while GitHub is slow or unset.
var List = mcpl.Def[struct{}, Connections]{
	Spec: mcpl.Spec{Summary: "List the organization's GitHub connections, and whether you may add one.", Access: mcpl.Member},
	Run: func(c *mcpl.Call, _ struct{}) (Connections, error) {
		out := Connections{Items: []Installation{}, CanConnect: c.Can("decide")}
		rows, err := c.DB.Query(c.Ctx, `SELECT id::text, account_login, account_type, suspended_at IS NOT NULL, created_at
		  FROM github_installations WHERE tenant_id=$1 AND deleted_at IS NULL ORDER BY created_at`, c.Tenant)
		if err != nil {
			return out, err
		}
		defer rows.Close()
		for rows.Next() {
			var i Installation
			if err := rows.Scan(&i.ID, &i.AccountLogin, &i.AccountType, &i.Suspended, &i.CreatedAt); err != nil {
				return out, err
			}
			out.Items = append(out.Items, i)
		}
		return out, rows.Err()
	},
}
