package catalog_service

import "shippin.cloud/console/internal/mcpl"

// CreateInput is a new catalog service: its id and its fields.
type CreateInput struct {
	ID string `json:"id"`
	Fields
}

// Create is shippin.services.catalog_service.create: POST /v1/operator/services/catalog-services.
var Create = mcpl.Def[CreateInput, Service]{
	Spec: mcpl.Spec{Summary: "Add a service to the catalog with its config schema, defaults and optional pinned deployment source and runtime contract.", Access: mcpl.Staff, Perm: "catalog.plans.write"},
	Run: func(c *mcpl.Call, in CreateInput) (Service, error) {
		if err := in.normalize(in.ID); err != nil {
			return Service{}, err
		}
		_, err := c.DB.Exec(c.Ctx, `INSERT INTO catalog_services(id,label,description,kind,docs_url,config_schema,default_config,deployment_source,runtime_contract,required,active)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, in.ID, in.Label, in.Description, in.Kind, in.DocsURL, in.ConfigSchema.raw(), in.DefaultConfig.raw(),
			jsonb(in.DeploymentSource), jsonb(in.RuntimeContract), in.Required, boolOr(in.Active))
		if err != nil {
			return Service{}, duplicate(err, in.ID)
		}
		return one(c, in.ID)
	},
}
