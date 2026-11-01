package catalog

import (
	"context"
	"encoding/json"
)

// Plan is a tier customers subscribe to (data, editable).
type Plan struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Rank        int    `json:"rank"`
	PriceCents  int    `json:"price_cents"`
	Public      bool   `json:"public"`
	Active      bool   `json:"active"`
}

const planCols = `id, label, description, rank, price_cents, public, active, updated_at`

func (s *Store) Plans(ctx context.Context) (json.RawMessage, error) {
	return s.listJSON(ctx, `SELECT `+planCols+` FROM plans ORDER BY rank, id`)
}

func (s *Store) Plan(ctx context.Context, id string) (json.RawMessage, error) {
	return s.oneJSON(ctx, `SELECT `+planCols+` FROM plans WHERE id=$1`, id)
}

// PlanPrice is the stored price (kept on updates by callers without catalog.prices.read).
func (s *Store) PlanPrice(ctx context.Context, id string) (int, error) {
	var n int
	if err := s.Pool.QueryRow(ctx, `SELECT price_cents FROM plans WHERE id=$1`, id).Scan(&n); err != nil {
		return 0, ErrNotFound
	}
	return n, nil
}

func (s *Store) CreatePlan(ctx context.Context, p Plan) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO plans(id,label,description,rank,price_cents,public,active) VALUES($1,$2,$3,$4,$5,$6,$7)`,
		p.ID, p.Label, p.Description, p.Rank, p.PriceCents, p.Public, p.Active)
	return err
}

func (s *Store) UpdatePlan(ctx context.Context, p Plan) error {
	return s.exec(ctx, `UPDATE plans SET label=$2, description=$3, rank=$4, price_cents=$5, public=$6, active=$7, updated_at=now() WHERE id=$1`,
		p.ID, p.Label, p.Description, p.Rank, p.PriceCents, p.Public, p.Active)
}

// DeletePlan refuses while tenants are still on the plan (the FK-free tenants.plan column).
func (s *Store) DeletePlan(ctx context.Context, id string) (int, error) {
	var n int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM tenants WHERE plan=$1`, id).Scan(&n); err != nil || n > 0 {
		return n, err
	}
	return 0, s.exec(ctx, `DELETE FROM plans WHERE id=$1`, id)
}
