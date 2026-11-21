package runner_job

import "shippin.cloud/console/internal/mcpl"

// ListInput narrows the list to one workspace.
type ListInput struct {
	Workspace string `query:"workspace" json:"-"`
}

// List is shippin.runtime.runner_job.list: GET /v1/tenants/{tenantId}/runtime/runner-jobs, the newest
// 200 first.
var List = mcpl.Def[ListInput, mcpl.Items[Job]]{
	Spec: mcpl.Spec{Summary: "List the organization's runner jobs, newest first.", Access: mcpl.Member},
	Run: func(c *mcpl.Call, in ListInput) (mcpl.Items[Job], error) {
		out := mcpl.Items[Job]{Items: []Job{}}
		rows, err := c.DB.Query(c.Ctx, `SELECT `+cols+` FROM shippin_runner_jobs
		  WHERE tenant_id=$1 AND ($2 = '' OR workspace_id::text = $2) ORDER BY queued_at DESC, id LIMIT 200`, c.Tenant, in.Workspace)
		if err != nil {
			return out, err
		}
		defer rows.Close()
		for rows.Next() {
			j, err := scan(rows)
			if err != nil {
				return out, err
			}
			out.Items = append(out.Items, j)
		}
		return out, rows.Err()
	},
}
