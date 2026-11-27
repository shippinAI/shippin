package guest

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/infrastructure/server"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/tenancy/orgs"
)

// AssignInput places one guest under a customer: an existing one by id, or the customer of the person
// with an email address, who gets a new customer when they own none.
type AssignInput struct {
	ByID
	// CustomerID is an existing customer. Give this or Email, not both.
	CustomerID string `json:"customer_id,omitempty"`
	// Email is the person the guest belongs to. Their customer is used; with none, one is created.
	Email string `json:"email,omitempty"`
	// Name names a customer created for Email; empty names it after the address, to be changed later.
	Name string `json:"name,omitempty"`
	// PlanID is the plan of a customer created for Email; empty uses the lowest active plan.
	PlanID string `json:"plan_id,omitempty"`
}

// owners is who each assigned guest on a server belongs to, by guest id. A call with no database, one
// made outside the runner, has no assignments to read.
func owners(c *mcpl.Call, serverID string) (map[string]hv.GuestOwner, error) {
	q := c.DB
	if q == nil {
		if c.Pool == nil {
			return nil, nil
		}
		q = c.Pool
	}
	rows, err := q.Query(c.Ctx, `SELECT o.guest, t.id::text, t.name FROM shippin_guest_owners o JOIN tenants t ON t.id=o.tenant_id WHERE o.server_id::text=$1`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]hv.GuestOwner{}
	for rows.Next() {
		var guest string
		var o hv.GuestOwner
		if err := rows.Scan(&guest, &o.CustomerID, &o.Name); err != nil {
			return nil, err
		}
		out[guest] = o
	}
	return out, rows.Err()
}

// Assign is shippin.infrastructure.server.guest.assign: POST /v1/operator/infrastructure/servers/{serverId}/guests/{guestId}/assign.
// It changes nothing on the hypervisor. A guest that was already running when the host was attached is
// placed this way; assigning again moves it to the other customer.
var Assign = mcpl.Def[AssignInput, hv.Guest]{
	Spec: mcpl.Spec{Summary: "Assign a guest to a customer: an existing one, or the customer of an email address, created when that person has none.", Access: mcpl.Staff, Perm: "customers.write"},
	Run: func(c *mcpl.Call, in AssignInput) (hv.Guest, error) {
		in.CustomerID, in.Email = strings.TrimSpace(in.CustomerID), strings.TrimSpace(in.Email)
		if (in.CustomerID == "") == (in.Email == "") {
			return hv.Guest{}, mcpl.Invalid("Name the customer by customer_id, or give the email address of the person the guest belongs to, not both.")
		}
		_, _, d, err := server.Open(c, in.Server)
		if err != nil {
			return hv.Guest{}, err
		}
		g, err := d.Guest(c.Ctx, in.ID)
		if err != nil {
			return hv.Guest{}, server.Problem(err)
		}
		tenant := in.CustomerID
		if in.Email != "" {
			tenant, _, err = orgs.CustomerForEmail(c.Ctx, c.DB, in.Email, in.Name, in.PlanID, c.Actor)
			if errors.Is(err, orgs.ErrBadInput) {
				return hv.Guest{}, mcpl.Invalid("Give a valid email address, and a plan that exists if you name one.")
			}
			if err != nil {
				return hv.Guest{}, err
			}
		}
		var o hv.GuestOwner
		err = c.DB.QueryRow(c.Ctx, `SELECT id::text, name FROM tenants WHERE kind='customer' AND id::text=$1`, tenant).Scan(&o.CustomerID, &o.Name)
		if errors.Is(err, pgx.ErrNoRows) {
			return hv.Guest{}, mcpl.Invalid("There is no customer with that id.")
		}
		if err != nil {
			return hv.Guest{}, err
		}
		if _, err := c.DB.Exec(c.Ctx, `INSERT INTO shippin_guest_owners(server_id,guest,tenant_id,assigned_by) VALUES($1::uuid,$2,$3::uuid,$4)
		  ON CONFLICT (server_id,guest) DO UPDATE SET tenant_id=EXCLUDED.tenant_id, assigned_by=EXCLUDED.assigned_by, assigned_at=now()`,
			in.Server, g.ID, o.CustomerID, c.Actor); err != nil {
			return hv.Guest{}, err
		}
		g.Self, g.Owner = IsSelf(g), &o
		return g, nil
	},
}
