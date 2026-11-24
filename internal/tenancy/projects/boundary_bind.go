package projects

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// ErrVMTaken: the tenant's VM already backs another boundary (one VM per tenant until multi-VM).
var ErrVMTaken = errors.New("tenant VM already backs another boundary")

// BoundaryRef names a boundary and its tenant.
type BoundaryRef struct{ ID, Tenant string }

// BoundaryState returns a boundary's state and Incus VM name ("" before provisioning).
func (s *Store) BoundaryState(ctx context.Context, tenant, id string) (state, vm string, err error) {
	err = s.Pool.QueryRow(ctx, `SELECT state, coalesce(legacy_vm,'') FROM boundaries WHERE id::text=$1 AND tenant_id::text=$2`,
		id, tenant).Scan(&state, &vm)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return state, vm, err
}

// SetBoundaryState records provisioning progress or an error (never touches ready/deleting rows).
func (s *Store) SetBoundaryState(ctx context.Context, tenant, id, state, msg string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE boundaries SET state=$3, error=nullif($4,''), updated_at=now()
	  WHERE id::text=$1 AND tenant_id::text=$2 AND state IN ('planned','provisioning','error')`, id, tenant, state, msg)
	return err
}

// BindVM records the provisioned VM on a planned boundary, marks it ready and places the tenant's
// pending projects on it, in one transaction. ErrVMTaken when another boundary already has the VM.
func (s *Store) BindVM(ctx context.Context, tenant, boundary, vm, cpu, mem, root, data string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var other string
	err = tx.QueryRow(ctx, `SELECT id::text FROM boundaries WHERE tenant_id::text=$1 AND legacy_vm=$2 AND id::text<>$3`,
		tenant, vm, boundary).Scan(&other)
	if err == nil {
		return ErrVMTaken
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE boundaries SET legacy_vm=$3, state='ready', error=NULL, cpu=$4, memory_mib=$5,
	  root_gib=$6, data_gib=$7, updated_at=now()
	  WHERE id::text=$1 AND tenant_id::text=$2 AND state IN ('planned','provisioning','error')`,
		boundary, tenant, vm, num(cpu), mib(mem), gib(root), gib(data))
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := placePending(ctx, tx, tenant, boundary); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// PlannedBoundaries lists boundaries waiting for a VM (planned, or failed and retryable).
func (s *Store) PlannedBoundaries(ctx context.Context) ([]BoundaryRef, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id::text, tenant_id::text FROM boundaries WHERE state IN ('planned','provisioning')`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (BoundaryRef, error) {
		var b BoundaryRef
		return b, r.Scan(&b.ID, &b.Tenant)
	})
}
