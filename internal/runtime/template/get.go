package template

import "shippin.cloud/console/internal/mcpl"

// Get is shippin.runtime.template.get: GET /v1/operator/runtime/templates/{templateId}.
var Get = mcpl.Def[ByID, Template]{
	Spec: mcpl.Spec{Summary: "Read one template with its image, size, repository, start command and ports.", Access: mcpl.Staff, Perm: "templates.read"},
	Run:  func(c *mcpl.Call, in ByID) (Template, error) { return one(c, in.ID) },
}
