package servers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"shippin.cloud/console/internal/ext/placement"
)

// fixedHosts is a placement policy that lets every organization use the same hosts.
type fixedHosts []string

func (f fixedHosts) Hosts(context.Context, placement.Querier, string) ([]string, error) {
	return f, nil
}

// A placement policy decides which platform hosts an organization sees and may use, with no
// assignment row at all; a host it may only use stays read-only.
func TestAPlacementPolicyDecidesTheHosts(t *testing.T) {
	s, ctx, tid := testStore(t)
	host, err := s.CreateEndpoint(ctx, Endpoint{ID: uuid.NewString(), Slug: "platform-1", Label: "Platform 1", Site: "main", Kind: "incus", BaseURL: "https://incus.invalid:8443"}, "u")
	if err != nil {
		t.Fatal(err)
	}
	if list, err := s.List(ctx, tid); err != nil || len(list) != 0 {
		t.Fatalf("without the policy the host is not the organization's: %+v %v", list, err)
	}

	s.Placement = fixedHosts{host.ID}
	if list, err := s.List(ctx, tid); err != nil || len(list) != 1 || list[0].ID != host.ID || !list[0].Estate {
		t.Fatalf("the policy's host must be listed: %+v %v", list, err)
	}
	if _, err := s.Get(ctx, tid, host.ID); err != nil {
		t.Fatalf("and readable: %v", err)
	}
	if e, err := s.VisibleEndpoint(ctx, tid, host.ID); err != nil || e.ID != host.ID {
		t.Fatalf("and its hypervisor visible: %+v %v", e, err)
	}
	if _, err := s.IssueToken(ctx, tid, host.ID, "u", time.Hour); err != ErrReadOnly {
		t.Fatalf("and read-only: %v", err)
	}

	s.Placement = fixedHosts{}
	if _, err := s.Get(ctx, tid, host.ID); err != ErrNotFound {
		t.Fatalf("a policy that takes the host away hides it again: %v", err)
	}
}

// Asked inside a transaction, today's policy holds the assignments it read until the transaction ends,
// so a host cannot be unassigned while a deployment is being placed on it.
func TestAssignedHoldsTheAssignmentsInATransaction(t *testing.T) {
	s, ctx, tid := testStore(t)
	host, err := s.CreateEndpoint(ctx, Endpoint{ID: uuid.NewString(), Slug: "platform-1", Label: "Platform 1", Site: "main", Kind: "incus", BaseURL: "https://incus.invalid:8443"}, "u")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Pool.Exec(ctx, `INSERT INTO server_assignments(server_id, tenant_id, role, created_by) VALUES($1,$2,'placement','u')`, host.ID, tid); err != nil {
		t.Fatal(err)
	}
	if hosts, err := (placement.Assigned{}).Hosts(ctx, s.Pool, tid); err != nil || len(hosts) != 1 || hosts[0] != host.ID {
		t.Fatalf("assigned hosts: %v %v", hosts, err)
	}

	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := (placement.Assigned{}).Hosts(ctx, tx, tid); err != nil {
		t.Fatal(err)
	}
	unassign := func() error {
		c, err := s.Pool.Begin(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = c.Rollback(ctx) }()
		if _, err := c.Exec(ctx, `SET LOCAL lock_timeout = '300ms'`); err != nil {
			return err
		}
		_, err = c.Exec(ctx, `DELETE FROM server_assignments WHERE server_id=$1 AND tenant_id=$2`, host.ID, tid)
		return err
	}
	var pg *pgconn.PgError
	if err := unassign(); !errors.As(err, &pg) || pg.Code != "55P03" {
		t.Fatalf("unassigning during the transaction must wait for it: %v", err)
	}
	_ = tx.Rollback(ctx)
	if err := unassign(); err != nil {
		t.Fatalf("after the transaction the host can be unassigned: %v", err)
	}
}
