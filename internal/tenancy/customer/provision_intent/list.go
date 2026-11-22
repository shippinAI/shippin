package provision_intent

import "shippin.cloud/console/internal/mcpl"

// List is shippin.tenancy.customer.provision_intent.list: GET /v1/operator/tenancy/customers/{customerId}/provision-intents.
var List = mcpl.Def[Parent, mcpl.Items[Intent]]{
	Spec: mcpl.Spec{Summary: "List a customer's latest provisioning requests, newest first.", Access: mcpl.Staff, Perm: "customers.read"},
	Run: func(c *mcpl.Call, in Parent) (mcpl.Items[Intent], error) {
		out := mcpl.Items[Intent]{Items: []Intent{}}
		if err := in.must(c); err != nil {
			return out, err
		}
		rows, err := c.DB.Query(c.Ctx, `SELECT `+cols+` FROM provision_intents WHERE tenant_id::text=$1 ORDER BY created_at DESC LIMIT 50`, in.CustomerID)
		if err != nil {
			return out, err
		}
		defer rows.Close()
		for rows.Next() {
			i, err := scan(rows)
			if err != nil {
				return out, err
			}
			out.Items = append(out.Items, i)
		}
		return out, rows.Err()
	},
}
