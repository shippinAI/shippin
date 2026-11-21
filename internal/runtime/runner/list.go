package runner

import "shippin.cloud/console/internal/mcpl"

// ListInput narrows the list to one state.
type ListInput struct {
	State State `query:"state" json:"-"`
}

// List is shippin.runtime.runner.list: GET /v1/operator/runtime/runners. What is not yet removed comes
// first, then the newest; at most 500.
var List = mcpl.Def[ListInput, mcpl.Items[Runner]]{
	Spec: mcpl.Spec{Summary: "List every runner machine, with its organization, job, host, state and age.", Access: mcpl.Staff, Perm: "infra.read"},
	Run: func(c *mcpl.Call, in ListInput) (mcpl.Items[Runner], error) {
		out := mcpl.Items[Runner]{Items: []Runner{}}
		if in.State != "" && !mcpl.Valid(in.State) {
			return out, mcpl.Invalid("There is no runner state " + string(in.State) + ".")
		}
		rows, err := c.DB.Query(c.Ctx, `SELECT `+cols+from+` WHERE ($1 = '' OR m.state = $1)
		  ORDER BY m.state = 'removed', m.created_at DESC, m.id LIMIT 500`, string(in.State))
		if err != nil {
			return out, err
		}
		defer rows.Close()
		for rows.Next() {
			r, err := scan(rows)
			if err != nil {
				return out, err
			}
			out.Items = append(out.Items, r)
		}
		return out, rows.Err()
	},
}
