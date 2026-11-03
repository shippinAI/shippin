package projects

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// AdoptVM records an existing tenant VM (vm-first `tvm-<tenant>`) as the tenant's first boundary
// (07: "Such a VM becomes the tenant's first boundary: its row stores that Incus name; nothing is
// renamed"), ready, sized from what hostd reports (Incus sizes with units). Idempotent per VM
// name, in one transaction with placing the tenant's pending projects on it.
func (s *Store) AdoptVM(ctx context.Context, tenant, vm, cpu, mem, root, data string) (string, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var id string
	err = tx.QueryRow(ctx, `SELECT id::text FROM boundaries WHERE tenant_id=$1 AND legacy_vm=$2 FOR UPDATE`, tenant, vm).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		nid, _ := uuid.NewV7()
		id = nid.String()
		_, err = tx.Exec(ctx, `INSERT INTO boundaries(id,tenant_id,state,cpu,memory_mib,root_gib,data_gib,legacy_vm)
		  VALUES($1,$2,'ready',$3,$4,$5,$6,$7)`, id, tenant, num(cpu), mib(mem), gib(root), gib(data), vm)
	} else if err == nil {
		_, err = tx.Exec(ctx, `UPDATE boundaries SET state='ready', error=NULL, updated_at=now() WHERE id=$1 AND state<>'deleting'`, id)
	}
	if err != nil {
		return "", err
	}
	if err = placePending(ctx, tx, tenant, id); err != nil {
		return "", err
	}
	return id, tx.Commit(ctx)
}

// MarkBoundaryReady flips a provisioned boundary to ready and places the tenant's pending
// projects (those waiting on it, and unplaced ones) on it.
func (s *Store) MarkBoundaryReady(ctx context.Context, tenant, boundary string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `UPDATE boundaries SET state='ready', error=NULL, updated_at=now()
	  WHERE id=$1 AND tenant_id=$2 AND state IN ('planned','provisioning','ready','error')`, boundary, tenant)
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

func placePending(ctx context.Context, tx pgx.Tx, tenant, boundary string) error {
	_, err := tx.Exec(ctx, `UPDATE projects SET boundary_id=$1, state='ready', updated_at=now()
	  WHERE tenant_id=$2 AND state='pending' AND (boundary_id IS NULL OR boundary_id=$1)`, boundary, tenant)
	return err
}

// num reads the leading integer of a CPU count ("16"); 1 when absent.
func num(s string) int {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	n, err := strconv.Atoi(s[:i])
	if err != nil || n < 1 {
		return 1
	}
	return n
}
