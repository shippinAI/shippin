// Package plan is the plan entity of the tenancy domain: what a customer subscribes to, with its price,
// the coverage tier it includes and the catalog services it includes (docs/plans.md). Staff with
// catalog.plans.write change plans; staff with catalog.plans.read read them, and the price only with
// catalog.prices.read. A plan's services are the nested entity in ./service.
package plan

import (
	"errors"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"shippin.cloud/console/internal/catalog"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/tenancy/coverage_tier"
)

// permPrices lets a caller see and set a plan's price.
const permPrices = "catalog.prices.read"

// Plan is a plan as staff read it, with the services it includes. The price is absent for a caller
// without catalog.prices.read.
type Plan struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Rank        int    `json:"rank"`
	PriceCents  *int   `json:"price_cents,omitempty"`
	Public      bool   `json:"public"`
	Active      bool   `json:"active"`
	// Coverage is the coverage tier the plan includes; an organization can be moved to another.
	Coverage  string    `json:"coverage"`
	UpdatedAt time.Time `json:"updated_at"`
	// Services are the plan's row of the plan x service matrix, by service id.
	Services []Service `json:"services"`
}

// Service is one cell of the plan x service matrix: whether the plan includes the service, the lock
// mode a customer's own setting meets, and the plan's limits (numbers are maximums under bounded).
type Service struct {
	PlanID    string         `json:"plan_id"`
	ServiceID string         `json:"service_id"`
	Enabled   bool           `json:"enabled"`
	Lock      catalog.Lock   `json:"lock"`
	Limits    map[string]any `json:"limits"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// Fields are what create and update take. Public and Active default to true, and an empty coverage to
// the lowest-ranked active tier. A price left out is 0 on create and kept on update. A caller without
// catalog.prices.read can't set a price on create, and keeps the stored one on update.
type Fields struct {
	Label       string `json:"label"`
	Description string `json:"description"`
	Rank        int    `json:"rank"`
	PriceCents  *int   `json:"price_cents,omitempty"`
	Public      *bool  `json:"public,omitempty"`
	Active      *bool  `json:"active,omitempty"`
	Coverage    string `json:"coverage"`
}

// ByID names one plan from the route.
type ByID struct {
	ID string `path:"planId" json:"-"`
}

var slug = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,39}$`)

// check fills the coverage default and refuses what a plan may not be, in the words the panel shows.
func (f *Fields) check(c *mcpl.Call, id string) error {
	if f.Coverage == "" {
		f.Coverage = coverage_tier.Lowest(c.Ctx, c.DB)
	}
	if !coverage_tier.Exists(c.Ctx, c.DB, f.Coverage) {
		return mcpl.Invalid("Pick the coverage tier this plan includes.")
	}
	if !slug.MatchString(id) || f.Label == "" || (f.PriceCents != nil && *f.PriceCents < 0) {
		return mcpl.Invalid("A plan needs a lowercase id (2-40 chars), a label and a non-negative price.")
	}
	return nil
}

func boolOr(b *bool) bool { return b == nil || *b }

const cols = `id, label, description, rank, price_cents, public, active, coalesce(coverage, ''), updated_at`

// read is the plans, lowest rank first, each with its services; id "" reads every plan. It is two
// queries however many plans there are.
func read(c *mcpl.Call, id string) ([]Plan, error) {
	rows, err := c.DB.Query(c.Ctx, `SELECT `+cols+` FROM plans WHERE $1 = '' OR id = $1 ORDER BY rank, id`, id)
	if err != nil {
		return nil, err
	}
	out, at, prices := []Plan{}, map[string]int{}, c.Can(permPrices)
	for rows.Next() {
		p := Plan{Services: []Service{}}
		var cents int
		if err := rows.Scan(&p.ID, &p.Label, &p.Description, &p.Rank, &cents, &p.Public, &p.Active, &p.Coverage, &p.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		if prices {
			p.PriceCents = &cents
		}
		at[p.ID] = len(out)
		out = append(out, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	cells, err := c.DB.Query(c.Ctx, `SELECT `+ServiceCols+` FROM plan_services WHERE $1 = '' OR plan_id = $1 ORDER BY plan_id, service_id`, id)
	if err != nil {
		return nil, err
	}
	defer cells.Close()
	for cells.Next() {
		s, err := ScanService(cells)
		if err != nil {
			return nil, err
		}
		if i, ok := at[s.PlanID]; ok {
			out[i].Services = append(out[i].Services, s)
		}
	}
	return out, cells.Err()
}

func one(c *mcpl.Call, id string) (Plan, error) {
	ps, err := read(c, id)
	if err != nil {
		return Plan{}, err
	}
	if len(ps) == 0 {
		return Plan{}, mcpl.NotFound("No plan " + id + ".")
	}
	return ps[0], nil
}

// ServiceCols are the plan_services columns ScanService reads, in its order.
const ServiceCols = `plan_id, service_id, enabled, lock, limits, updated_at`

// ScanService reads one plan x service cell.
func ScanService(row pgx.Row) (Service, error) {
	var s Service
	err := row.Scan(&s.PlanID, &s.ServiceID, &s.Enabled, &s.Lock, &s.Limits, &s.UpdatedAt)
	if s.Limits == nil {
		s.Limits = map[string]any{}
	}
	return s, err
}

// Exists reports whether a plan with this id exists.
func Exists(c *mcpl.Call, id string) (bool, error) {
	var ok bool
	err := c.DB.QueryRow(c.Ctx, `SELECT EXISTS(SELECT 1 FROM plans WHERE id=$1)`, id).Scan(&ok)
	return ok, err
}

// taken reports a unique violation: the id is in use.
func taken(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}
