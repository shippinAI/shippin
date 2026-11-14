package customer

import "shippin.cloud/console/internal/mcpl"

// Get is shippin.tenancy.customer.get: GET /v1/operator/tenancy/customers/{customerId}.
var Get = mcpl.Def[ByID, Customer]{
	Spec: mcpl.Spec{Summary: "Read one customer.", Access: mcpl.Staff, Perm: "customers.read"},
	Run:  func(c *mcpl.Call, in ByID) (Customer, error) { return one(c, in.ID) },
}
