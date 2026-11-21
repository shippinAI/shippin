package service

import "shippin.cloud/console/internal/mcpl"

// List is shippin.tenancy.customer.service.list: GET /v1/operator/tenancy/customers/{customerId}/services.
var List = mcpl.Def[Parent, mcpl.Items[Override]]{
	Spec: mcpl.Spec{Summary: "List a customer's own settings for catalog services, over what its plan gives.", Access: mcpl.Staff, Perm: "customers.read"},
	Run: func(c *mcpl.Call, in Parent) (mcpl.Items[Override], error) {
		out := mcpl.Items[Override]{Items: []Override{}}
		if err := in.must(c); err != nil {
			return out, err
		}
		rows, err := c.DB.Query(c.Ctx, `SELECT `+cols+` FROM tenant_services WHERE tenant_id::text=$1 ORDER BY service_id`, in.CustomerID)
		if err != nil {
			return out, err
		}
		defer rows.Close()
		for rows.Next() {
			o, err := scan(rows)
			if err != nil {
				return out, err
			}
			out.Items = append(out.Items, o)
		}
		return out, rows.Err()
	},
}
