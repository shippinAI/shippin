package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"shippin.cloud/console/internal/projects"
)

// Tenant detail for the provider panel: the tenant's Nomen (Zitadel) org members with their org
// roles, the Shippin owner rows from the DB, and its projects. Read only; the IdP call uses the
// console's read-only operator token and returns no secrets.
type opMember struct {
	UserID string   `json:"user_id"`
	Name   string   `json:"name"`
	Email  string   `json:"email"`
	Roles  []string `json:"roles"`
}

type opOwner struct {
	AccountID string    `json:"account_id"`
	Email     string    `json:"email"`
	Since     time.Time `json:"since"`
}

func (a *API) orgMembers(ctx context.Context, org string) ([]opMember, error) {
	var res struct {
		Result []struct {
			UserID      string   `json:"userId"`
			DisplayName string   `json:"displayName"`
			Email       string   `json:"email"`
			Roles       []string `json:"roles"`
		} `json:"result"`
	}
	body := map[string]any{"query": map[string]any{"limit": a.cfg.Operator.MemberLimit}}
	if err := a.flow.IdPRead(ctx, a.cfg.IDPReadToken, "POST", "/management/v1/orgs/me/members/_search", org, body, &res); err != nil {
		return []opMember{}, err
	}
	out := make([]opMember, 0, len(res.Result))
	for _, m := range res.Result {
		out = append(out, opMember{UserID: m.UserID, Name: m.DisplayName, Email: m.Email, Roles: m.Roles})
	}
	return out, nil
}

func (a *API) tenantOwners(ctx context.Context, tenant string) ([]opOwner, error) {
	rows, err := a.db.Pool.Query(ctx, `SELECT a.id::text, a.email, o.created_at FROM tenant_owners o
	  JOIN accounts a ON a.id=o.account_id WHERE o.tenant_id=$1 ORDER BY o.created_at`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []opOwner{}
	for rows.Next() {
		var o opOwner
		if err := rows.Scan(&o.AccountID, &o.Email, &o.Since); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (a *API) opTenantDetail(w http.ResponseWriter, r *http.Request, _ *caller) {
	t, ok := a.opTenant(w, r)
	if !ok {
		return
	}
	ctx, errs := r.Context(), []string{}
	members := []opMember{}
	if t.IdpOrgID != "" {
		var err error
		if members, err = a.orgMembers(ctx, t.IdpOrgID); err != nil {
			errs = append(errs, a.cfg.IDP.Name+" members: "+err.Error())
		}
	}
	owners, err := a.tenantOwners(ctx, t.ID)
	if err != nil {
		errs, owners = append(errs, "owners: store unavailable"), []opOwner{}
	}
	teams, err := a.db.Teams(ctx, t.ID)
	if err != nil {
		errs = append(errs, "teams: store unavailable")
	}
	projs, err := (&projects.Store{Pool: a.db.Pool}).Projects(ctx, t.ID)
	if err != nil {
		errs, projs = append(errs, "projects: store unavailable"), json.RawMessage("[]")
	}
	writeJSON(w, map[string]any{"tenant": t, "org": map[string]string{"id": t.IdpOrgID, "name": t.Name},
		"members": members, "owners": owners, "teams": teams, "projects": projs, "errors": errs})
}
