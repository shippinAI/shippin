package catalog_service

import "shippin.cloud/console/internal/mcpl"

// UpdateInput is a service's new definition. Every field is replaced: a source or contract left out is removed.
type UpdateInput struct {
	ByID
	Fields
}

// Update is shippin.services.catalog_service.update: PUT /v1/operator/services/catalog-services/{catalogServiceId}.
var Update = mcpl.Def[UpdateInput, Service]{
	Spec: mcpl.Spec{Summary: "Replace a catalog service's definition: label, config schema, defaults, deployment source and runtime contract.", Access: mcpl.Staff, Perm: "catalog.plans.write"},
	Run: func(c *mcpl.Call, in UpdateInput) (Service, error) {
		if err := in.normalize(in.ID); err != nil {
			return Service{}, err
		}
		tag, err := c.DB.Exec(c.Ctx, `UPDATE catalog_services SET label=$2, description=$3, kind=$4, docs_url=$5, config_schema=$6,
			default_config=$7, deployment_source=$8, runtime_contract=$9, required=$10, active=$11, updated_at=now() WHERE id=$1`,
			in.ID, in.Label, in.Description, in.Kind, in.DocsURL, in.ConfigSchema.raw(), in.DefaultConfig.raw(),
			jsonb(in.DeploymentSource), jsonb(in.RuntimeContract), in.Required, boolOr(in.Active))
		if err != nil {
			return Service{}, err
		}
		if tag.RowsAffected() == 0 {
			return Service{}, mcpl.NotFound("No catalog service " + in.ID + ".")
		}
		return one(c, in.ID)
	},
}
