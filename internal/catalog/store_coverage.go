package catalog

import (
	"context"
	"encoding/json"
	"slices"
)

// CoverageTier is a level of support and operation sold beside a plan (data, editable). Response
// targets are first-response minutes counted within SupportHours; nil means no target.
type CoverageTier struct {
	ID              string   `json:"id"`
	Label           string   `json:"label"`
	Description     string   `json:"description"`
	Rank            int      `json:"rank"`
	SignIn          string   `json:"sign_in"`
	SupportHours    string   `json:"support_hours"`
	Channels        []string `json:"channels"`
	CriticalMinutes *int     `json:"critical_minutes"`
	HighMinutes     *int     `json:"high_minutes"`
	NormalMinutes   *int     `json:"normal_minutes"`
	// UptimeTarget is a percentage such as 99.9; nil means no uptime commitment.
	UptimeTarget      *float64 `json:"uptime_target"`
	Operations        string   `json:"operations"`
	NamedContact      bool     `json:"named_contact"`
	Reviews           string   `json:"reviews"`
	TrainingHours     int      `json:"training_hours"`
	Consulting        string   `json:"consulting"`
	CustomDeployments string   `json:"custom_deployments"`
	RemoteOperations  string   `json:"remote_operations"`
	TeamTraining      string   `json:"team_training"`
	// The charge is the greater of PriceCents a month and SpendPercent of the organization's plan charges.
	PriceCents   int     `json:"price_cents"`
	SpendPercent float64 `json:"spend_percent"`
	Public       bool    `json:"public"`
	Active       bool    `json:"active"`
}

// Allowed values, kept in step with the CHECK constraints in migration 070.
var (
	SignInRules     = []string{"password", "second_factor", "identity_provider"}
	SupportHours    = []string{"business", "critical_always", "always"}
	SupportChannels = []string{"docs", "community", "email", "chat", "phone", "video"}
	OperationModes  = []string{"self", "assisted", "managed"}
	ReviewCadences  = []string{"none", "quarterly", "monthly"}
	ServiceTerms    = []string{"none", "quoted", "included"}
)

const coverageCols = `id, label, description, rank, sign_in, support_hours, channels, critical_minutes, high_minutes, normal_minutes,
	uptime_target::float8 AS uptime_target, operations, named_contact, reviews, training_hours, consulting, custom_deployments,
	remote_operations, team_training, price_cents, spend_percent::float8 AS spend_percent, public, active, updated_at`

// coverageCustomerCols is what a customer sees of its own tier: the service, not the price or catalog state.
const coverageCustomerCols = `id, label, description, support_hours, channels, critical_minutes, high_minutes, normal_minutes,
	uptime_target::float8 AS uptime_target, operations, named_contact, reviews, training_hours, consulting, custom_deployments,
	remote_operations, team_training`

func (s *Store) CoverageTiers(ctx context.Context) (json.RawMessage, error) {
	return s.listJSON(ctx, `SELECT `+coverageCols+` FROM coverage_tiers ORDER BY rank, id`)
}

func (s *Store) CoverageTier(ctx context.Context, id string) (json.RawMessage, error) {
	return s.oneJSON(ctx, `SELECT `+coverageCols+` FROM coverage_tiers WHERE id=$1`, id)
}

// TenantCoverage is the organization's own tier (its override, else its plan's), as the customer sees it.
func (s *Store) TenantCoverage(ctx context.Context, tenantID string) (json.RawMessage, error) {
	return s.oneJSON(ctx, `SELECT `+coverageCustomerCols+` FROM coverage_tiers WHERE id = (
	  SELECT coalesce(t.coverage, p.coverage) FROM tenants t LEFT JOIN plans p ON p.id=t.plan WHERE t.id::text=$1)`, tenantID)
}

// Coverage is an organization's effective tier and the sign-in rule it sets.
type Coverage struct {
	Tier   string
	SignIn string
}

// CoverageOf maps tenant ids to their effective coverage. A tenant with none is absent.
func (s *Store) CoverageOf(ctx context.Context, tenantIDs []string) (map[string]Coverage, error) {
	out := map[string]Coverage{}
	rows, err := s.Pool.Query(ctx, `SELECT t.id::text, c.id, c.sign_in FROM tenants t LEFT JOIN plans p ON p.id=t.plan
	  JOIN coverage_tiers c ON c.id = coalesce(t.coverage, p.coverage) WHERE t.id::text = ANY($1)`, tenantIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var c Coverage
		if err := rows.Scan(&id, &c.Tier, &c.SignIn); err != nil {
			return nil, err
		}
		out[id] = c
	}
	return out, rows.Err()
}

// CoveragePrice is the stored charge (kept on updates by callers without catalog.prices.read).
func (s *Store) CoveragePrice(ctx context.Context, id string) (int, float64, error) {
	var cents int
	var pct float64
	if err := s.Pool.QueryRow(ctx, `SELECT price_cents, spend_percent::float8 FROM coverage_tiers WHERE id=$1`, id).Scan(&cents, &pct); err != nil {
		return 0, 0, ErrNotFound
	}
	return cents, pct, nil
}

func (s *Store) CoverageExists(ctx context.Context, id string) bool {
	var ok bool
	_ = s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM coverage_tiers WHERE id=$1)`, id).Scan(&ok)
	return ok
}

// LowestCoverage is the lowest-ranked active tier, for plans created without one.
func (s *Store) LowestCoverage(ctx context.Context) string {
	var id string
	_ = s.Pool.QueryRow(ctx, `SELECT id FROM coverage_tiers WHERE active ORDER BY rank, id LIMIT 1`).Scan(&id)
	return id
}

const coverageArgs = `$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23`

func (c CoverageTier) args() []any {
	return []any{c.ID, c.Label, c.Description, c.Rank, c.SignIn, c.SupportHours, c.Channels, c.CriticalMinutes, c.HighMinutes,
		c.NormalMinutes, c.UptimeTarget, c.Operations, c.NamedContact, c.Reviews, c.TrainingHours, c.Consulting, c.CustomDeployments,
		c.RemoteOperations, c.TeamTraining, c.PriceCents, c.SpendPercent, c.Public, c.Active}
}

func (s *Store) CreateCoverage(ctx context.Context, c CoverageTier) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO coverage_tiers (id, label, description, rank, sign_in, support_hours, channels, critical_minutes,
	  high_minutes, normal_minutes, uptime_target, operations, named_contact, reviews, training_hours, consulting, custom_deployments,
	  remote_operations, team_training, price_cents, spend_percent, public, active) VALUES ($1,`+coverageArgs+`)`, c.args()...)
	return err
}

func (s *Store) UpdateCoverage(ctx context.Context, c CoverageTier) error {
	return s.exec(ctx, `UPDATE coverage_tiers SET (label, description, rank, sign_in, support_hours, channels, critical_minutes, high_minutes,
	  normal_minutes, uptime_target, operations, named_contact, reviews, training_hours, consulting, custom_deployments, remote_operations,
	  team_training, price_cents, spend_percent, public, active, updated_at) = (`+coverageArgs+`, now()) WHERE id=$1`, c.args()...)
}

// DeleteCoverage refuses while a plan includes the tier; organizations moved to it fall back to their plan's.
func (s *Store) DeleteCoverage(ctx context.Context, id string) (int, error) {
	var n int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM plans WHERE coverage=$1`, id).Scan(&n); err != nil || n > 0 {
		return n, err
	}
	return 0, s.exec(ctx, `DELETE FROM coverage_tiers WHERE id=$1`, id)
}

func oneOf(v string, allowed []string) bool { return slices.Contains(allowed, v) }
