// Package customer is the operator's catalog view of an organization: the plan it is on, the coverage
// it is moved to, who to reach and where, and its provisioning state. Staff with customers.write change
// it; staff with customers.read read it. It is not membership: who belongs to an organization and what
// they may do there is the organization's own. Its actions are the other files in this folder; a
// customer's own service settings, effective configuration and provisioning requests are the nested
// entities service, effective_config and provision_intent.
package customer

import (
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/tenancy/coverage_tier"
)

// Field-level permissions: a caller without them neither sees nor sets these fields.
const (
	permContacts = "customers.contacts.read" // contacts and endpoints
	permNotes    = "customers.notes.read"
)

// unlinkedOrg marks a customer created here before its sign-in organization exists; provisioning links it.
const unlinkedOrg = "unlinked:"

// Contact is someone to reach at the customer.
type Contact struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

// Endpoint is where the customer's systems are reached; kind is lan, tailnet or public by convention.
type Endpoint struct {
	Kind string `json:"kind"`
	Host string `json:"host"`
	Note string `json:"note"`
}

// Customer is an organization as the operator's catalog sees it. Contacts and endpoints are absent for a
// caller without customers.contacts.read, notes without customers.notes.read.
type Customer struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	PlanID string `json:"plan_id"`
	// Status is the organization's lifecycle (draft, provisioning, active, suspended, error); it changes
	// through provisioning, never here.
	Status    string      `json:"status"`
	Contacts  *[]Contact  `json:"contacts,omitempty"`
	Endpoints *[]Endpoint `json:"endpoints,omitempty"`
	Notes     *string     `json:"notes,omitempty"`
	// Coverage moves the organization off its plan's coverage tier; null keeps the plan's.
	// CoverageEffective is the tier that applies.
	Coverage          *string `json:"coverage"`
	CoverageEffective *string `json:"coverage_effective"`
	// IdpLinked is whether its sign-in organization exists yet.
	IdpLinked  bool      `json:"idp_linked"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Workspaces int       `json:"workspaces"`
	// LastIntent is the status of its latest provisioning request, if any.
	LastIntent *string `json:"last_intent"`
}

// Fields are what create and update take. Contacts, endpoints and notes left out are empty on create
// and, for a caller who may not see them, kept on update. An empty or null coverage uses the plan's.
type Fields struct {
	Name      string     `json:"name"`
	PlanID    string     `json:"plan_id"`
	Coverage  *string    `json:"coverage,omitempty"`
	Contacts  []Contact  `json:"contacts,omitempty"`
	Endpoints []Endpoint `json:"endpoints,omitempty"`
	Notes     string     `json:"notes,omitempty"`
}

// ByID names one customer from the route.
type ByID struct {
	ID string `path:"customerId" json:"-"`
}

// check trims and validates the fields; the message says what to fix.
func (f *Fields) check(c *mcpl.Call) error {
	f.Name = strings.TrimSpace(f.Name)
	if f.Contacts == nil {
		f.Contacts = []Contact{}
	}
	if f.Endpoints == nil {
		f.Endpoints = []Endpoint{}
	}
	if f.Coverage != nil && *f.Coverage == "" {
		f.Coverage = nil
	}
	if f.Name == "" || len(f.Name) > 120 {
		return mcpl.Invalid("A customer needs a name of up to 120 characters.")
	}
	for _, x := range f.Contacts {
		if x.Email != "" && !strings.Contains(x.Email, "@") {
			return mcpl.Invalid("Contact e-mail " + x.Email + " doesn't look like an address.")
		}
	}
	var plan bool
	if err := c.DB.QueryRow(c.Ctx, `SELECT EXISTS(SELECT 1 FROM plans WHERE id=$1)`, f.PlanID).Scan(&plan); err != nil {
		return err
	}
	if !plan {
		return mcpl.Invalid("Pick a plan that exists in Plans & Tiers.")
	}
	if f.Coverage != nil && !coverage_tier.Exists(c.Ctx, c.Pool, *f.Coverage) {
		return mcpl.Invalid("Pick a coverage tier that exists, or leave it empty to use the plan's.")
	}
	return nil
}

// Exists reports whether id is a customer; the nested entities answer 404 for one that is not.
func Exists(c *mcpl.Call, id string) (bool, error) {
	var ok bool
	err := c.DB.QueryRow(c.Ctx, `SELECT EXISTS(SELECT 1 FROM tenants WHERE kind='customer' AND id::text=$1)`, id).Scan(&ok)
	return ok, err
}

// NotFound is the refusal for an id that names no customer.
func NotFound() error { return mcpl.NotFound("Customer not found.") }

const cols = `t.id::text, t.name, t.plan, t.status, t.contacts, t.endpoints, t.notes, t.coverage,
	coalesce(t.coverage, (SELECT p.coverage FROM plans p WHERE p.id=t.plan)),
	NOT starts_with(t.idp_org_id, '` + unlinkedOrg + `'), t.created_at, t.updated_at,
	(SELECT count(*) FROM workspaces w WHERE w.tenant_id=t.id),
	(SELECT status FROM provision_intents p WHERE p.tenant_id=t.id ORDER BY created_at DESC LIMIT 1)`

// scan reads one row of cols, with contacts, endpoints and notes only for a caller who may see them.
func scan(c *mcpl.Call, row pgx.Row) (Customer, error) {
	var x Customer
	var contacts []Contact
	var endpoints []Endpoint
	var notes string
	err := row.Scan(&x.ID, &x.Name, &x.PlanID, &x.Status, &contacts, &endpoints, &notes, &x.Coverage, &x.CoverageEffective,
		&x.IdpLinked, &x.CreatedAt, &x.UpdatedAt, &x.Workspaces, &x.LastIntent)
	if err != nil {
		return x, err
	}
	if c.Can(permContacts) {
		if contacts == nil {
			contacts = []Contact{}
		}
		if endpoints == nil {
			endpoints = []Endpoint{}
		}
		x.Contacts, x.Endpoints = &contacts, &endpoints
	}
	if c.Can(permNotes) {
		x.Notes = &notes
	}
	return x, nil
}

func one(c *mcpl.Call, id string) (Customer, error) {
	x, err := scan(c, c.DB.QueryRow(c.Ctx, `SELECT `+cols+` FROM tenants t WHERE t.kind='customer' AND t.id::text=$1`, id))
	if err == pgx.ErrNoRows {
		return x, NotFound()
	}
	return x, err
}
