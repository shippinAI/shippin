package customer

import "shippin.cloud/console/internal/mcpl"

// List is shippin.tenancy.customer.list: GET /v1/operator/tenancy/customers.
var List = mcpl.Def[struct{}, mcpl.Items[Customer]]{
	Spec: mcpl.Spec{Summary: "List the customers: each organization's plan, coverage, contacts and provisioning state, oldest first.", Access: mcpl.Staff, Perm: "customers.read"},
	Run: func(c *mcpl.Call, _ struct{}) (mcpl.Items[Customer], error) {
		out := mcpl.Items[Customer]{Items: []Customer{}}
		rows, err := c.DB.Query(c.Ctx, `SELECT `+cols+` FROM tenants t WHERE t.kind='customer' ORDER BY t.created_at`)
		if err != nil {
			return out, err
		}
		defer rows.Close()
		for rows.Next() {
			x, err := scan(c, rows)
			if err != nil {
				return out, err
			}
			out.Items = append(out.Items, x)
		}
		return out, rows.Err()
	},
}
