package catalog_service

import "shippin.cloud/console/internal/mcpl"

// Get is shippin.services.catalog_service.get: GET /v1/operator/services/catalog-services/{catalogServiceId}.
var Get = mcpl.Def[ByID, Service]{
	Spec: mcpl.Spec{Summary: "Read one catalog service definition with its config schema, defaults and pinned deployment source.", Access: mcpl.Staff, Perm: "catalog.plans.read"},
	Run:  func(c *mcpl.Call, in ByID) (Service, error) { return one(c, in.ID) },
}
