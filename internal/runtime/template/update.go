package template

import (
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/platform/config"
)

// UpdateInput is a template's new fields.
type UpdateInput struct {
	ByID
	Fields
}

// Update is shippin.runtime.template.update: PUT /v1/operator/runtime/templates/{templateId}.
// Development spaces already made from it keep what they were made with.
var Update = mcpl.Def[UpdateInput, Template]{
	Spec: mcpl.Spec{Summary: "Change a template. Development spaces already made from it are not changed.", Access: mcpl.Staff, Perm: "templates.write"},
	Run: func(c *mcpl.Call, in UpdateInput) (Template, error) {
		rules, err := mcpl.Need[config.Workspaces](c)
		if err != nil {
			return Template{}, err
		}
		t := Template{ID: in.ID, Fields: in.Fields}
		if msg := Check(&t, rules); msg != "" {
			return Template{}, mcpl.Invalid(msg)
		}
		tag, err := c.DB.Exec(c.Ctx, `UPDATE templates SET slug=$2, name=$3, description=$4, icon=$5, image=$6, toolchain=$7, cpu=$8,
		  memory_gib=$9, disk_gib=$10, repo=$11, startup=$12, ports=$13, enabled=$14, sort=$15, updated_at=now() WHERE id::text=$1`,
			t.ID, t.Slug, t.Name, t.Description, t.Icon, t.Image, t.Toolchain, t.CPU, t.MemoryGiB, t.DiskGiB, t.Repo, t.Startup, t.Ports, t.Enabled, t.Sort)
		if err != nil {
			return Template{}, err
		}
		if tag.RowsAffected() == 0 {
			return Template{}, mcpl.NotFound("No template " + in.ID + ".")
		}
		return one(c, t.ID)
	},
}
