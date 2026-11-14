// Package coverage_tier is the coverage tier entity of the tenancy domain: support and operation sold
// beside a plan (docs/plans.md, Coverage). The entity's types, storage and rules live here; each action
// is its own file (list.go, get.go, create.go, update.go, delete.go), found by cmd/mcplgen.
package coverage_tier

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"shippin.cloud/console/internal/mcpl"
)

// Permissions this entity uses.
const (
	permRead   = "catalog.plans.read"
	permWrite  = "catalog.plans.write"
	permPrices = "catalog.prices.read"
)

// The choices a tier is made of. Each type owns its values, kept in step with the CHECK constraints in
// migration 070; the panel's types and the OpenAPI document list them from here.
type (
	// SignIn is the least a member must sign in with.
	SignIn string
	// Hours are when support answers.
	Hours string
	// Channel is a way to reach support.
	Channel string
	// Mode is who runs day-to-day operations.
	Mode string
	// Cadence is how often the organization's setup is reviewed with it.
	Cadence string
	// Term is how an extra service comes with the tier.
	Term string
)

func (SignIn) Values() []string { return []string{"password", "second_factor", "identity_provider"} }
func (Hours) Values() []string  { return []string{"business", "critical_always", "always"} }
func (Channel) Values() []string {
	return []string{"docs", "community", "email", "chat", "phone", "video"}
}
func (Mode) Values() []string    { return []string{"self", "assisted", "managed"} }
func (Cadence) Values() []string { return []string{"none", "quarterly", "monthly"} }
func (Term) Values() []string    { return []string{"none", "quoted", "included"} }

// Tier is a coverage tier as staff read it. Response targets are first-reply minutes within the support
// hours; nil means no target. The price is absent for a caller without catalog.prices.read.
type Tier struct {
	ID              string    `json:"id"`
	Label           string    `json:"label"`
	Description     string    `json:"description"`
	Rank            int       `json:"rank"`
	SignIn          SignIn    `json:"sign_in"`
	SupportHours    Hours     `json:"support_hours"`
	Channels        []Channel `json:"channels"`
	CriticalMinutes *int      `json:"critical_minutes"`
	HighMinutes     *int      `json:"high_minutes"`
	NormalMinutes   *int      `json:"normal_minutes"`
	// UptimeTarget is a percentage such as 99.9; nil means no uptime commitment.
	UptimeTarget      *float64 `json:"uptime_target"`
	Operations        Mode     `json:"operations"`
	NamedContact      bool     `json:"named_contact"`
	Reviews           Cadence  `json:"reviews"`
	TrainingHours     int      `json:"training_hours"`
	Consulting        Term     `json:"consulting"`
	CustomDeployments Term     `json:"custom_deployments"`
	RemoteOperations  Term     `json:"remote_operations"`
	TeamTraining      Term     `json:"team_training"`
	// The charge is the greater of PriceCents a month and SpendPercent of the organization's plan charges.
	PriceCents   *int      `json:"price_cents,omitempty"`
	SpendPercent *float64  `json:"spend_percent,omitempty"`
	Public       bool      `json:"public"`
	Active       bool      `json:"active"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Fields are what create and update take. Empty choices take their defaults; Public and Active default to
// true. A price left out keeps the stored one, which is how a caller who may not see prices saves.
type Fields struct {
	Label             string    `json:"label"`
	Description       string    `json:"description"`
	Rank              int       `json:"rank"`
	SignIn            SignIn    `json:"sign_in"`
	SupportHours      Hours     `json:"support_hours"`
	Channels          []Channel `json:"channels"`
	CriticalMinutes   *int      `json:"critical_minutes"`
	HighMinutes       *int      `json:"high_minutes"`
	NormalMinutes     *int      `json:"normal_minutes"`
	UptimeTarget      *float64  `json:"uptime_target"`
	Operations        Mode      `json:"operations"`
	NamedContact      bool      `json:"named_contact"`
	Reviews           Cadence   `json:"reviews"`
	TrainingHours     int       `json:"training_hours"`
	Consulting        Term      `json:"consulting"`
	CustomDeployments Term      `json:"custom_deployments"`
	RemoteOperations  Term      `json:"remote_operations"`
	TeamTraining      Term      `json:"team_training"`
	PriceCents        *int      `json:"price_cents,omitempty"`
	SpendPercent      *float64  `json:"spend_percent,omitempty"`
	Public            *bool     `json:"public,omitempty"`
	Active            *bool     `json:"active,omitempty"`
}

// ByID names one tier in the route.
type ByID struct {
	ID string `path:"coverageTierId" json:"-"`
}

func or[T ~string](v, d T) T {
	if v == "" {
		return d
	}
	return v
}

// normalize fills defaults, then checks every value; the message names the field so a form can say what is wrong.
func (f *Fields) normalize() error {
	f.SignIn, f.SupportHours, f.Operations, f.Reviews = or(f.SignIn, "password"), or(f.SupportHours, "business"), or(f.Operations, "self"), or(f.Reviews, "none")
	f.Consulting, f.CustomDeployments = or(f.Consulting, "quoted"), or(f.CustomDeployments, "quoted")
	f.RemoteOperations, f.TeamTraining = or(f.RemoteOperations, "none"), or(f.TeamTraining, "quoted")
	if f.Channels == nil {
		f.Channels = []Channel{}
	}
	switch {
	case f.Label == "" || len(f.Label) > 80:
		return mcpl.Invalid("A coverage tier needs a label of up to 80 characters.")
	case !mcpl.Valid(f.SignIn):
		return mcpl.Invalid("Sign-in must be password, second_factor or identity_provider.")
	case !mcpl.Valid(f.SupportHours):
		return mcpl.Invalid("Support hours must be business, critical_always or always.")
	case !mcpl.Valid(f.Operations):
		return mcpl.Invalid("Operations must be self, assisted or managed.")
	case !mcpl.Valid(f.Reviews):
		return mcpl.Invalid("Reviews must be none, quarterly or monthly.")
	case !mcpl.Valid(f.Consulting) || !mcpl.Valid(f.CustomDeployments) || !mcpl.Valid(f.RemoteOperations) || !mcpl.Valid(f.TeamTraining):
		return mcpl.Invalid("Consulting, custom deployments, remote operations and team training must each be none, quoted or included.")
	case f.TrainingHours < 0 || f.TrainingHours > 10000:
		return mcpl.Invalid("Training hours must be between 0 and 10000 a year.")
	case (f.PriceCents != nil && *f.PriceCents < 0) || (f.SpendPercent != nil && (*f.SpendPercent < 0 || *f.SpendPercent > 100)):
		return mcpl.Invalid("The price must not be negative, and the share of plan charges must be between 0 and 100 percent.")
	case f.UptimeTarget != nil && (*f.UptimeTarget <= 0 || *f.UptimeTarget >= 100):
		return mcpl.Invalid("An uptime target is a percentage below 100, such as 99.9.")
	}
	for _, m := range []*int{f.CriticalMinutes, f.HighMinutes, f.NormalMinutes} {
		if m != nil && *m <= 0 {
			return mcpl.Invalid("A response target is a number of minutes above zero, or empty for none.")
		}
	}
	for _, ch := range f.Channels {
		if !mcpl.Valid(ch) {
			return mcpl.Invalid("Channels are docs, community, email, chat, phone and video.")
		}
	}
	return nil
}

func boolOr(b *bool) bool { return b == nil || *b }

const cols = `id, label, description, rank, sign_in, support_hours, channels, critical_minutes, high_minutes, normal_minutes,
	uptime_target::float8, operations, named_contact, reviews, training_hours, consulting, custom_deployments,
	remote_operations, team_training, price_cents, spend_percent::float8, public, active, updated_at`

func scan(row pgx.Row, prices bool) (Tier, error) {
	var t Tier
	var cents int
	var pct float64
	err := row.Scan(&t.ID, &t.Label, &t.Description, &t.Rank, &t.SignIn, &t.SupportHours, &t.Channels, &t.CriticalMinutes, &t.HighMinutes,
		&t.NormalMinutes, &t.UptimeTarget, &t.Operations, &t.NamedContact, &t.Reviews, &t.TrainingHours, &t.Consulting, &t.CustomDeployments,
		&t.RemoteOperations, &t.TeamTraining, &cents, &pct, &t.Public, &t.Active, &t.UpdatedAt)
	if prices {
		t.PriceCents, t.SpendPercent = &cents, &pct
	}
	return t, err
}

func one(c *mcpl.Call, id string) (Tier, error) {
	t, err := scan(c.DB.QueryRow(c.Ctx, `SELECT `+cols+` FROM coverage_tiers WHERE id=$1`, id), c.Can(permPrices))
	if err == pgx.ErrNoRows {
		return t, mcpl.NotFound("No coverage tier " + id + ".")
	}
	return t, err
}

// Exists reports whether a tier with this id exists.
func Exists(ctx context.Context, pool *pgxpool.Pool, id string) bool {
	var ok bool
	_ = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM coverage_tiers WHERE id=$1)`, id).Scan(&ok)
	return ok
}

// Lowest is the lowest-ranked active tier, for plans created without one.
func Lowest(ctx context.Context, pool *pgxpool.Pool) string {
	var id string
	_ = pool.QueryRow(ctx, `SELECT id FROM coverage_tiers WHERE active ORDER BY rank, id LIMIT 1`).Scan(&id)
	return id
}

// Coverage is an organization's effective tier and the sign-in rule it sets.
type Coverage struct {
	Tier   string
	SignIn string
}

// Of maps tenant ids to their effective coverage (their own, else their plan's). A tenant with none is absent.
func Of(ctx context.Context, pool *pgxpool.Pool, tenantIDs []string) (map[string]Coverage, error) {
	out := map[string]Coverage{}
	rows, err := pool.Query(ctx, `SELECT t.id::text, c.id, c.sign_in FROM tenants t LEFT JOIN plans p ON p.id=t.plan
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

// CustomerView is what a member sees of their organization's tier: the service, not the price or catalog state.
type CustomerView struct {
	ID                string    `json:"id"`
	Label             string    `json:"label"`
	Description       string    `json:"description"`
	SupportHours      Hours     `json:"support_hours"`
	Channels          []Channel `json:"channels"`
	CriticalMinutes   *int      `json:"critical_minutes"`
	HighMinutes       *int      `json:"high_minutes"`
	NormalMinutes     *int      `json:"normal_minutes"`
	UptimeTarget      *float64  `json:"uptime_target"`
	Operations        Mode      `json:"operations"`
	NamedContact      bool      `json:"named_contact"`
	Reviews           Cadence   `json:"reviews"`
	TrainingHours     int       `json:"training_hours"`
	Consulting        Term      `json:"consulting"`
	CustomDeployments Term      `json:"custom_deployments"`
	RemoteOperations  Term      `json:"remote_operations"`
	TeamTraining      Term      `json:"team_training"`
}

// ForTenant is the organization's own tier (its override, else its plan's); nil when it has none.
func ForTenant(ctx context.Context, q mcpl.DB, tenantID string) (*CustomerView, error) {
	var v CustomerView
	err := q.QueryRow(ctx, `SELECT id, label, description, support_hours, channels, critical_minutes, high_minutes, normal_minutes,
	  uptime_target::float8, operations, named_contact, reviews, training_hours, consulting, custom_deployments, remote_operations, team_training
	  FROM coverage_tiers WHERE id = (SELECT coalesce(t.coverage, p.coverage) FROM tenants t LEFT JOIN plans p ON p.id=t.plan WHERE t.id::text=$1)`, tenantID).
		Scan(&v.ID, &v.Label, &v.Description, &v.SupportHours, &v.Channels, &v.CriticalMinutes, &v.HighMinutes, &v.NormalMinutes, &v.UptimeTarget,
			&v.Operations, &v.NamedContact, &v.Reviews, &v.TrainingHours, &v.Consulting, &v.CustomDeployments, &v.RemoteOperations, &v.TeamTraining)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}
