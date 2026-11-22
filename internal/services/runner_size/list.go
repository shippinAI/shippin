package runner_size

import "shippin.cloud/console/internal/mcpl"

// List is shippin.services.runner_size.list: GET /v1/operator/services/runner-sizes, smallest first.
var List = mcpl.Def[struct{}, mcpl.Items[Size]]{
	Spec: mcpl.Spec{Summary: "List the runner sizes customers' jobs can ask for.", Access: mcpl.Staff, Perm: "catalog.plans.read"},
	Run: func(c *mcpl.Call, _ struct{}) (mcpl.Items[Size], error) {
		out := mcpl.Items[Size]{Items: []Size{}}
		rows, err := c.DB.Query(c.Ctx, `SELECT `+cols+` FROM shippin_runner_sizes ORDER BY cores, memory_mib, id`)
		if err != nil {
			return out, err
		}
		defer rows.Close()
		for rows.Next() {
			s, err := scan(rows)
			if err != nil {
				return out, err
			}
			out.Items = append(out.Items, s)
		}
		return out, rows.Err()
	},
}
