package orgs

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestLocalCustomerCreatesDefaultWorkspaceAndOnlyChosenMembership(t *testing.T) {
	s, ctx, _ := testStore(t)
	owner, staff := uuid.NewString(), uuid.NewString()
	for _, id := range []string{owner, staff} {
		if _, err := s.Pool.Exec(ctx, `INSERT INTO identity_users(id,email,name,role) VALUES($1,$2,'Person','member')`, id, id+"@example.invalid"); err != nil {
			t.Fatal(err)
		}
	}
	var plan string
	if err := s.Pool.QueryRow(ctx, `SELECT id FROM plans WHERE active ORDER BY rank LIMIT 1`).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateLocalCustomer(ctx, "Customer", "missing-plan", owner, "local:"+staff); !errors.Is(err, ErrBadInput) {
		t.Fatal("invalid plan", err)
	}
	if _, err := s.CreateLocalCustomer(ctx, "Customer", plan, uuid.NewString(), "local:"+staff); !errors.Is(err, ErrNotFound) {
		t.Fatal("missing owner", err)
	}
	tenant, err := s.CreateLocalCustomer(ctx, "Customer", plan, owner, "local:"+staff)
	if err != nil {
		t.Fatal(err)
	}
	var assigned, actorGranted, workspace, audited bool
	var status string
	if err := s.Pool.QueryRow(ctx, `SELECT status,
 EXISTS(SELECT 1 FROM identity_members m WHERE m.org_id::text=t.idp_org_id AND m.user_id=$2 AND m.role='owner'),
 EXISTS(SELECT 1 FROM identity_members m WHERE m.org_id::text=t.idp_org_id AND m.user_id=$3),
 EXISTS(SELECT 1 FROM shippin_workspaces w WHERE w.tenant_id=t.id AND w.is_default),
 EXISTS(SELECT 1 FROM identity_audit a WHERE a.action='organization.created' AND a.detail->>'tenant_id'=t.id::text)
 FROM tenants t WHERE id=$1`, tenant, owner, staff).Scan(&status, &assigned, &actorGranted, &workspace, &audited); err != nil {
		t.Fatal(err)
	}
	if status != "draft" || !assigned || actorGranted || !workspace || !audited {
		t.Fatalf("status=%s assigned=%v actorGranted=%v default=%v audited=%v", status, assigned, actorGranted, workspace, audited)
	}
}
