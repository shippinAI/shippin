package deployments

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// RecordRuntimeUnavailable records that desired state reached the worker but no
// supported application executor is installed. It intentionally performs no
// provider I/O. The generation lock and event check make lease replays safe.
func (s *Store) RecordRuntimeUnavailable(ctx context.Context, tenantID, deploymentID, actor string, expectedGeneration int64) (bool, error) {
	if !validUUID(tenantID) || !validUUID(deploymentID) || expectedGeneration < 1 || len(actor) == 0 ||
		len(actor) > 255 || strings.TrimSpace(actor) != actor || strings.ContainsAny(actor, "\r\n\x00") {
		return false, ErrInvalidRequest
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var generation int64
	err = tx.QueryRow(ctx, `SELECT desired_generation FROM shippin_deployments
		WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, deploymentID).Scan(&generation)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, err
	}
	if generation != expectedGeneration {
		return false, tx.Commit(ctx)
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM shippin_deployment_events
		WHERE tenant_id=$1 AND deployment_id=$2 AND generation=$3
		AND event_type=$4 AND failure_code='runtime_unsupported')`, tenantID, deploymentID,
		generation, EventOperationFailed).Scan(&exists); err != nil {
		return false, err
	}
	if exists {
		return false, tx.Commit(ctx)
	}
	eventID, err := uuid.NewV7()
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE shippin_deployments SET observed_state=$3,updated_by=$4,updated_at=now()
		WHERE tenant_id=$1 AND id=$2`, tenantID, deploymentID, ObservedFailed, actor); err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO shippin_deployment_events
		(id,deployment_id,tenant_id,generation,event_type,observed_state,failure_code,actor)
		VALUES($1,$2,$3,$4,$5,$6,'runtime_unsupported',$7)`, eventID, deploymentID, tenantID,
		generation, EventOperationFailed, ObservedFailed, actor); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}
