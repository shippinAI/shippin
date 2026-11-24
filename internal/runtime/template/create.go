package template

import (
	"github.com/google/uuid"

	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/platform/config"
)

// Create is shippin.runtime.template.create: POST /v1/operator/runtime/templates.
var Create = mcpl.Def[Fields, Template]{
	Spec: mcpl.Spec{Summary: "Add a template: an image, a default size, and optionally a repository, a start command and ports.", Access: mcpl.Staff, Perm: "templates.write"},
	Run: func(c *mcpl.Call, in Fields) (Template, error) {
		rules, err := mcpl.Need[config.Workspaces](c)
		if err != nil {
			return Template{}, err
		}
		id, err := uuid.NewV7()
		if err != nil {
			return Template{}, err
		}
		t := Template{ID: id.String(), Fields: in}
		if msg := Check(&t, rules); msg != "" {
			return Template{}, mcpl.Invalid(msg)
		}
		if err := insert(c.Ctx, c.DB, t); err != nil {
			return Template{}, err
		}
		return one(c, t.ID)
	},
}
