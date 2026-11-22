package catalog_service

import "shippin.cloud/console/internal/mcpl"

// List is shippin.services.catalog_service.list: GET /v1/operator/services/catalog-services.
var List = mcpl.Def[mcpl.None, mcpl.Items[Service]]{
	Spec: mcpl.Spec{Summary: "List the catalog's service definitions, required ones first.", Access: mcpl.Staff, Perm: "catalog.plans.read"},
	Run: func(c *mcpl.Call, _ mcpl.None) (mcpl.Items[Service], error) {
		out := mcpl.Items[Service]{Items: []Service{}}
		rows, err := c.DB.Query(c.Ctx, `SELECT `+cols+` FROM catalog_services ORDER BY required DESC, id`)
		if err != nil {
			return out, err
		}
		defer rows.Close()
		for rows.Next() {
			s, err := scan(rows)
			if err != nil {
				return out, err
			}
			out.Items = append(out.Items, s)
		}
		return out, rows.Err()
	},
}
