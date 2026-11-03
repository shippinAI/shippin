package catalog

import (
	"context"
	"encoding/json"
)

// Customer is the admin view of a tenant row plus catalog fields.
type Customer struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Plan      string          `json:"plan_id"`
	Status    string          `json:"status"`
	Contacts  json.RawMessage `json:"contacts"`
	Endpoints json.RawMessage `json:"endpoints"`
	Notes     string          `json:"notes"`
}

// unlinkedOrg marks a customer created here before its IdP org exists (provisioning links it).
const unlinkedOrg = "unlinked:"

const customerCols = `t.id::text AS id, t.name, t.plan AS plan_id, t.status, t.contacts, t.endpoints, t.notes,
	NOT starts_with(t.idp_org_id, '` + unlinkedOrg + `') AS idp_linked, t.created_at, t.updated_at,
	(SELECT count(*) FROM workspaces w WHERE w.tenant_id=t.id) AS workspaces,
	(SELECT status FROM provision_intents p WHERE p.tenant_id=t.id ORDER BY created_at DESC LIMIT 1) AS last_intent`

func (s *Store) Customers(ctx context.Context) (json.RawMessage, error) {
	return s.listJSON(ctx, `SELECT `+customerCols+` FROM tenants t WHERE t.kind='customer' ORDER BY t.created_at`)
}

func (s *Store) Customer(ctx context.Context, id string) (json.RawMessage, error) {
	return s.oneJSON(ctx, `SELECT `+customerCols+` FROM tenants t WHERE t.kind='customer' AND t.id::text=$1`, id)
}

// CustomerRow reads the editable fields (updates keep the ones the caller can't see).
func (s *Store) CustomerRow(ctx context.Context, id string) (Customer, error) {
	c := Customer{ID: id}
	err := s.Pool.QueryRow(ctx, `SELECT name, plan, status, contacts, endpoints, notes FROM tenants WHERE kind='customer' AND id::text=$1`, id).
		Scan(&c.Name, &c.Plan, &c.Status, &c.Contacts, &c.Endpoints, &c.Notes)
	if err != nil {
		return c, ErrNotFound
	}
	return c, nil
}

func (s *Store) CustomerPlan(ctx context.Context, id string) (string, error) {
	var p string
	err := s.Pool.QueryRow(ctx, `SELECT plan FROM tenants WHERE kind='customer' AND id::text=$1`, id).Scan(&p)
	if err != nil {
		return "", ErrNotFound
	}
	return p, nil
}

func (s *Store) CreateCustomer(ctx context.Context, c Customer) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO tenants(id,idp_org_id,name,plan,status,contacts,endpoints,notes) VALUES($1,$2,$3,$4,'draft',$5,$6,$7)`,
		c.ID, unlinkedOrg+c.ID, c.Name, c.Plan, c.Contacts, c.Endpoints, c.Notes)
	return err
}

func (s *Store) UpdateCustomer(ctx context.Context, c Customer) error {
	// Status is not editable here: lifecycle goes through provision/suspend/resume.
	return s.exec(ctx, `UPDATE tenants SET name=$2, plan=$3, contacts=$4, endpoints=$5, notes=$6, updated_at=now() WHERE kind='customer' AND id::text=$1`,
		c.ID, c.Name, c.Plan, c.Contacts, c.Endpoints, c.Notes)
}

// DeleteCustomer removes only drafts that were never linked to an IdP org and own no workspaces;
// live tenants go through the audited operator delete.
func (s *Store) DeleteCustomer(ctx context.Context, id string) error {
	return s.exec(ctx, `DELETE FROM tenants t WHERE t.kind='customer' AND t.id::text=$1 AND t.status='draft' AND starts_with(t.idp_org_id,$2)
		AND NOT EXISTS (SELECT 1 FROM workspaces w WHERE w.tenant_id=t.id)`, id, unlinkedOrg)
}

func (s *Store) PlanExists(ctx context.Context, id string) bool {
	var ok bool
	_ = s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM plans WHERE id=$1)`, id).Scan(&ok)
	return ok
}
