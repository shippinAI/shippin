package template

import "shippin.cloud/console/internal/mcpl"

// Delete is shippin.runtime.template.delete: DELETE /v1/operator/runtime/templates/{templateId}.
// Development spaces made from it keep running.
var Delete = mcpl.Def[ByID, mcpl.None]{
	Spec: mcpl.Spec{Summary: "Remove a template. Development spaces made from it keep running.", Access: mcpl.Staff, Perm: "templates.write"},
	Run: func(c *mcpl.Call, in ByID) (mcpl.None, error) {
		tag, err := c.DB.Exec(c.Ctx, `DELETE FROM templates WHERE id::text=$1`, in.ID)
		if err == nil && tag.RowsAffected() == 0 {
			return mcpl.None{}, mcpl.NotFound("No template " + in.ID + ".")
		}
		return mcpl.None{}, err
	},
}
