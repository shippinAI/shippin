package deployments

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// RecordOperationEventWithLease appends one reconciler outcome while the exact
// deployment-generation job lease is live. State changes and event history
// commit together. This method does not perform provider I/O.
func (s *Store) RecordOperationEventWithLease(ctx context.Context, tenantID, deploymentID string,
	jobID, leaseOwner string, event DeploymentEvent) (DeploymentEvent, error) {
	eventID, err := uuid.NewV7()
	if err != nil {
		return DeploymentEvent{}, err
	}
	event.ID = eventID.String()
	if !validUUID(tenantID) || !validUUID(deploymentID) || !validUUID(jobID) ||
		strings.TrimSpace(leaseOwner) == "" || len(leaseOwner) > 255 ||
		!validUUID(event.IdempotencyKey) || event.TenantID != tenantID || event.DeploymentID != deploymentID ||
		!workerOperationEvent(event.Type) || event.ReviewProvenance != nil || event.Validate() != nil {
		return DeploymentEvent{}, ErrInvalidRequest
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return DeploymentEvent{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	targetID := deploymentID + ":" + strconv.FormatInt(event.Generation, 10)
	var owned string
	err = tx.QueryRow(ctx, `SELECT id::text FROM jobs WHERE id=$1 AND tenant_id=$2
		AND kind='deployment.reconcile' AND target_type='deployment_generation' AND target_id=$3
		AND status='running' AND lease_owner=$4 AND lease_until>now() FOR UPDATE`,
		jobID, tenantID, targetID, leaseOwner).Scan(&owned)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeploymentEvent{}, ErrLostJobLease
	}
	if err != nil {
		return DeploymentEvent{}, err
	}
	var generation int64
	err = tx.QueryRow(ctx, `SELECT desired_generation FROM shippin_deployments
		WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, deploymentID).Scan(&generation)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeploymentEvent{}, ErrNotFound
	}
	if err != nil {
		return DeploymentEvent{}, err
	}
	if generation != event.Generation {
		return DeploymentEvent{}, ErrGenerationConflict
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, tenantID+":"+event.IdempotencyKey); err != nil {
		return DeploymentEvent{}, err
	}
	var prior DeploymentEvent
	var eventType, observed, health, failure string
	err = tx.QueryRow(ctx, `SELECT id::text,deployment_id::text,tenant_id::text,generation,event_type,
		coalesce(observed_state,''),coalesce(provider_operation_id,''),coalesce(health_status,''),
		health_duration_ms,coalesce(failure_code,''),actor,recorded_at
		FROM shippin_deployment_events WHERE tenant_id=$1 AND idempotency_key=$2`, tenantID, event.IdempotencyKey).
		Scan(&prior.ID, &prior.DeploymentID, &prior.TenantID, &prior.Generation, &eventType,
			&observed, &prior.ProviderOperationID, &health, &prior.HealthDurationMS, &failure, &prior.Actor, &prior.RecordedAt)
	if err == nil {
		prior.Type, prior.ObservedState, prior.HealthStatus, prior.FailureCode = EventType(eventType), ObservedState(observed), health, failure
		if sameOperationEvent(prior, event) {
			if err := tx.Commit(ctx); err != nil {
				return DeploymentEvent{}, err
			}
			return prior, nil
		}
		return DeploymentEvent{}, ErrIdempotencyConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return DeploymentEvent{}, err
	}
	if event.ObservedState != "" {
		if _, err := tx.Exec(ctx, `UPDATE shippin_deployments SET observed_state=$3,updated_by=$4,updated_at=now()
			WHERE tenant_id=$1 AND id=$2`, tenantID, deploymentID, event.ObservedState, event.Actor); err != nil {
			return DeploymentEvent{}, err
		}
	}
	err = tx.QueryRow(ctx, `INSERT INTO shippin_deployment_events
		(id,deployment_id,tenant_id,generation,event_type,observed_state,provider_operation_id,health_status,
		health_duration_ms,failure_code,actor,idempotency_key)
		VALUES($1,$2,$3,$4,$5,NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),$9,NULLIF($10,''),$11,$12)
		RETURNING recorded_at`, event.ID, deploymentID, tenantID, event.Generation, event.Type,
		string(event.ObservedState), event.ProviderOperationID, event.HealthStatus, event.HealthDurationMS,
		event.FailureCode, event.Actor, event.IdempotencyKey).Scan(&event.RecordedAt)
	if err != nil {
		return DeploymentEvent{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DeploymentEvent{}, err
	}
	return event, nil
}

func workerOperationEvent(eventType EventType) bool {
	switch eventType {
	case EventOperationStarted, EventProviderObserved, EventHealthObserved, EventOperationFailed, EventOperationCancelled:
		return true
	default:
		return false
	}
}

func sameOperationEvent(a, b DeploymentEvent) bool {
	return a.DeploymentID == b.DeploymentID && a.TenantID == b.TenantID && a.Generation == b.Generation &&
		a.Type == b.Type && a.ObservedState == b.ObservedState && a.ProviderOperationID == b.ProviderOperationID &&
		a.HealthStatus == b.HealthStatus && equalDuration(a.HealthDurationMS, b.HealthDurationMS) &&
		a.FailureCode == b.FailureCode && a.Actor == b.Actor
}

func equalDuration(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
