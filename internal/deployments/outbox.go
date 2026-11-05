package deployments

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrOutboxNotFound = errors.New("deployment outbox record not found")
	ErrOutboxConflict = errors.New("deployment outbox record was already published differently")
)

// OutboxMessage is an identifier-only wake-up request. The worker reloads all
// deployment configuration and review state from the owning stores.
type OutboxMessage struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	DeploymentID string    `json:"deployment_id"`
	Generation   int64     `json:"generation"`
	CreatedBy    string    `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
}

func insertOutbox(ctx context.Context, tx pgx.Tx, tenantID, deploymentID string, generation int64, actor string) error {
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO shippin_deployment_outbox
		(id,tenant_id,deployment_id,generation,created_by)
		VALUES($1,$2,$3,$4,$5)`, id, tenantID, deploymentID, generation, actor)
	return err
}

// PendingOutbox returns unpublished generation wake-ups in creation order.
// Publishing is at least once: callers enqueue idempotently, then mark the row.
func (s *Store) PendingOutbox(ctx context.Context, limit int) ([]OutboxMessage, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `SELECT id::text,tenant_id::text,deployment_id::text,generation,created_by,created_at
		FROM shippin_deployment_outbox WHERE published_at IS NULL
		ORDER BY created_at,tenant_id,deployment_id,generation,id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]OutboxMessage, 0)
	for rows.Next() {
		var item OutboxMessage
		if err := rows.Scan(&item.ID, &item.TenantID, &item.DeploymentID, &item.Generation, &item.CreatedBy, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// MarkOutboxPublished records the existing jobs runner's job ID after enqueue.
// If the relay crashes between enqueue and this write, it may enqueue again;
// the live-job uniqueness constraint makes that replay safe.
func (s *Store) MarkOutboxPublished(ctx context.Context, outboxID, jobID string) error {
	if !validUUID(outboxID) || !validUUID(jobID) {
		return ErrInvalidRequest
	}
	tag, err := s.pool.Exec(ctx, `UPDATE shippin_deployment_outbox SET job_id=$2,published_at=now()
		WHERE id=$1 AND published_at IS NULL`, outboxID, jobID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	var current string
	err = s.pool.QueryRow(ctx, `SELECT coalesce(job_id::text,'') FROM shippin_deployment_outbox WHERE id=$1`, outboxID).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrOutboxNotFound
	}
	if err != nil {
		return err
	}
	if current == jobID {
		return nil
	}
	return ErrOutboxConflict
}
