package github_repository

import (
	"strconv"

	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/ports"
)

// List is shippin.integrations.github_repository.list: GET /v1/tenants/{tenantId}/integrations/github-repositories.
// It is every repository the organization's installations reach now, with its assignment, plus any
// assigned repository GitHub no longer lists, so a lost one is visible rather than silently gone.
var List = mcpl.Def[struct{}, mcpl.Items[Repository]]{
	Spec: mcpl.Spec{Summary: "List the organization's GitHub repositories and where each is assigned.", Access: mcpl.Member},
	Run: func(c *mcpl.Call, _ struct{}) (mcpl.Items[Repository], error) {
		out := mcpl.Items[Repository]{Items: []Repository{}}
		g, err := mcpl.Need[ports.GitHub](c)
		if err != nil {
			return out, err
		}
		insts, err := installations(c)
		if err != nil {
			return out, err
		}
		assigned := map[string]Repository{}
		rows, err := c.DB.Query(c.Ctx, `SELECT `+cols+` FROM shippin_github_repositories WHERE tenant_id=$1 ORDER BY full_name`, c.Tenant)
		if err != nil {
			return out, err
		}
		var order []string
		for rows.Next() {
			r, err := scan(rows)
			if err != nil {
				rows.Close()
				return out, err
			}
			assigned[r.ID] = r
			order = append(order, r.ID)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return out, err
		}
		seen := map[string]bool{}
		for _, inst := range insts {
			rs, err := g.Repositories(c.Ctx, inst)
			if err != nil {
				return out, mcpl.Conflict("GitHub didn't list this organization's repositories just now. Try again.")
			}
			for _, gr := range rs {
				id := strconv.FormatInt(gr.ID, 10)
				if seen[id] {
					continue
				}
				seen[id] = true
				r, ok := assigned[id]
				if !ok {
					r = Repository{ID: id, InstallationID: strconv.FormatInt(inst, 10)}
				}
				r.FullName, r.Private = gr.FullName, gr.Private
				out.Items = append(out.Items, r)
			}
		}
		for _, id := range order {
			if !seen[id] {
				out.Items = append(out.Items, assigned[id])
			}
		}
		return out, nil
	},
}
