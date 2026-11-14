package service

import (
	"bytes"
	"encoding/json"

	"shippin.cloud/console/internal/catalog"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/tenancy/plan"
)

// Update is shippin.tenancy.plan.service.update: PUT /v1/operator/tenancy/plans/{planId}/services/{serviceId}.
// It sets the cell whether or not the plan included the service before.
var Update = mcpl.Def[Cell, plan.Service]{
	Spec: mcpl.Spec{Summary: "Include a catalog service in a plan, or change its lock mode and limits there.", Access: mcpl.Staff, Perm: "catalog.plans.write"},
	Run: func(c *mcpl.Call, in Cell) (plan.Service, error) {
		if in.Lock == "" {
			in.Lock = catalog.LockBounded
		}
		if raw := bytes.TrimSpace(in.Limits); len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
			in.Limits = json.RawMessage(`{}`)
		}
		var m map[string]any
		if !mcpl.Valid(in.Lock) || json.Unmarshal(in.Limits, &m) != nil || m == nil {
			return plan.Service{}, mcpl.Invalid("Lock must be locked, overridable, mergeable or bounded; limits must be an object.")
		}
		if ok, err := plan.Exists(c, in.PlanID); err != nil || !ok {
			return plan.Service{}, orNotFound(err, "No plan "+in.PlanID+".")
		}
		var known bool
		if err := c.DB.QueryRow(c.Ctx, `SELECT EXISTS(SELECT 1 FROM catalog_services WHERE id=$1)`, in.ID).Scan(&known); err != nil || !known {
			return plan.Service{}, orNotFound(err, "No catalog service "+in.ID+".")
		}
		enabled := in.Enabled == nil || *in.Enabled
		return plan.ScanService(c.DB.QueryRow(c.Ctx, `INSERT INTO plan_services(plan_id,service_id,enabled,lock,limits) VALUES($1,$2,$3,$4,$5)
		  ON CONFLICT (plan_id,service_id) DO UPDATE SET enabled=$3, lock=$4, limits=$5, updated_at=now()
		  RETURNING `+plan.ServiceCols, in.PlanID, in.ID, enabled, in.Lock, in.Limits))
	},
}

// orNotFound is err, or a 404 with msg when the lookup worked and found nothing.
func orNotFound(err error, msg string) error {
	if err != nil {
		return err
	}
	return mcpl.NotFound(msg)
}
