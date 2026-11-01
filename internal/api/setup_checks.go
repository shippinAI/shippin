package api

import (
	"context"
	"encoding/json"

	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/managed"
	"shippin.cloud/console/internal/projects"
)

// Setup checklist per tenant: real state read from the DB and config, never assumed. Each check
// says ok | todo | off (not enabled for this tenant) | error, and how to fix it.
type setupCheck struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	State   string `json:"state"`
	Detail  string `json:"detail,omitempty"`
	Project string `json:"project_id,omitempty"`
	Fix     string `json:"fix,omitempty"` // ui hint: org | boundary | rrflow_enable | rrflow_attach | devspace
}

type setupProject struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Workspaces int    `json:"workspaces"`
}

func (a *API) setupChecks(ctx context.Context, t *db.Tenant) ([]setupCheck, error) {
	out := []setupCheck{{ID: "org", Label: a.cfg.IDP.Name + " organization", State: "todo", Detail: "no organization yet", Fix: "org"}}
	if t.IdpOrgID != "" {
		out[0].State, out[0].Detail, out[0].Fix = "ok", t.IdpOrgID, ""
	}
	var bState string
	err := a.db.Pool.QueryRow(ctx, `SELECT coalesce((SELECT state FROM boundaries WHERE tenant_id=$1 AND state<>'deleting'
	  ORDER BY created_at LIMIT 1),'')`, t.ID).Scan(&bState)
	if err != nil {
		return nil, err
	}
	b := setupCheck{ID: "boundary", Label: "Tenant VM (boundary)", State: "todo", Detail: "no boundary planned", Fix: "boundary"}
	switch bState {
	case "ready":
		b.State, b.Detail, b.Fix = "ok", "ready", ""
	case "error":
		b.State, b.Detail = "error", "provisioning failed"
	case "":
	default:
		b.Detail = bState
	}
	out = append(out, b)
	on := a.svcs != nil && a.cfg.Services.RRFlow.Allows(t.ID)
	en := setupCheck{ID: "rrflow", Label: "rrflow enabled for this tenant", State: "off", Detail: "services.rrflow rollout excludes this tenant", Fix: "rrflow_enable"}
	if a.svcs == nil {
		en.Detail, en.Fix = "rrflow schema (migration 026) not applied yet", ""
	}
	if on {
		en.State, en.Detail, en.Fix = "ok", "", ""
	}
	out = append(out, en, autoAttachCheck(on, a.rrflowAutoAttach(ctx, t.ID)))
	if on {
		vms, err := a.svcs.VMInstances(ctx, t.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, vmRRFlowCheck(vms))
	}
	raw, err := (&projects.Store{Pool: a.db.Pool}).Projects(ctx, t.ID)
	if err != nil {
		return nil, err
	}
	var ps []setupProject
	_ = json.Unmarshal(raw, &ps)
	for _, p := range ps {
		var insts []managed.Instance
		if a.svcs != nil {
			if insts, err = a.svcs.ByProject(ctx, t.ID, p.ID); err != nil {
				return nil, err
			}
		}
		ch := rrflowCheck(p, insts, on)
		if ch.Fix == "rrflow_attach" && ch.State == "todo" && a.rrflowAutoAttach(ctx, t.ID) {
			ch.Detail += "; attaches automatically with the next devspace"
		}
		out = append(out, ch)
	}
	return out, nil
}
