package template

import "shippin.cloud/console/internal/mcpl"

// List is shippin.runtime.template.list: GET /v1/operator/runtime/templates. Every template, the ones
// turned off too, in the order the create page shows them.
var List = mcpl.Def[struct{}, mcpl.Items[Template]]{
	Spec: mcpl.Spec{Summary: "List every template a development space can start from, including the ones turned off.", Access: mcpl.Staff, Perm: "templates.read"},
	Run: func(c *mcpl.Call, _ struct{}) (mcpl.Items[Template], error) {
		ts, err := all(c.Ctx, c.DB, false)
		return mcpl.Items[Template]{Items: ts}, err
	},
}
