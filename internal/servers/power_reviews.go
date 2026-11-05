package servers

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrPowerReviewConflict = errors.New("servers: guest power review conflicts with existing action")

// PowerReview is Shippin's correlation and one-shot execution record. AgentHub
// remains authoritative for approval, policy and reviewer state.
type PowerReview struct {
	ID             string          `json:"id"`
	ServerID       string          `json:"server_id"`
	Guest          string          `json:"guest"`
	Verb           string          `json:"verb"`
	Provider       string          `json:"provider"`
	EndpointSHA256 string          `json:"endpoint_sha256"`
	ActionSHA256   string          `json:"action_sha256"`
	IdempotencyKey string          `json:"-"`
	AgentHubTicket string          `json:"agenthub_ticket_id"`
	TicketSHA256   string          `json:"ticket_sha256,omitempty"`
	State          string          `json:"state"`
	FailureCode    string          `json:"failure_code,omitempty"`
	Provenance     json.RawMessage `json:"provenance,omitempty"`
	Actor          string          `json:"-"`
	ActorName      string          `json:"actor_name,omitempty"`
	Note           string          `json:"note"`
}

const powerReviewColumns = `id::text,server_id::text,guest,verb,provider,endpoint_sha256,action_sha256,idempotency_key::text,agenthub_ticket_id,coalesce(ticket_sha256,''),state,coalesce(failure_code,''),provenance,actor,actor_name,change_note`

func scanPowerReview(row pgx.Row) (PowerReview, error) {
	var v PowerReview
	err := row.Scan(&v.ID, &v.ServerID, &v.Guest, &v.Verb, &v.Provider, &v.EndpointSHA256, &v.ActionSHA256,
		&v.IdempotencyKey, &v.AgentHubTicket, &v.TicketSHA256, &v.State, &v.FailureCode, &v.Provenance,
		&v.Actor, &v.ActorName, &v.Note)
	if errors.Is(err, pgx.ErrNoRows) {
		return PowerReview{}, ErrNotFound
	}
	return v, err
}

// ReservePowerReview creates the durable request before calling AgentHub. Reusing
// a key for a different action is rejected; the same action resumes its record.
func (s *Store) ReservePowerReview(ctx context.Context, v PowerReview) (PowerReview, bool, error) {
	if uuid.Validate(v.ID) != nil || uuid.Validate(v.ServerID) != nil || uuid.Validate(v.IdempotencyKey) != nil ||
		v.Guest == "" || v.Actor == "" || v.Note == "" || (v.Verb != "start" && v.Verb != "stop" && v.Verb != "shutdown" && v.Verb != "reboot") {
		return PowerReview{}, false, ErrIdentityInvalid
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return PowerReview{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `INSERT INTO shippin_infra_power_reviews
		(id,server_id,guest,verb,provider,endpoint_sha256,action_sha256,idempotency_key,agenthub_ticket_id,state,actor,actor_name,change_note)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'preparing',$10,$11,$12)
		ON CONFLICT (server_id,idempotency_key) DO NOTHING`, v.ID, v.ServerID, v.Guest, v.Verb, v.Provider,
		v.EndpointSHA256, v.ActionSHA256, v.IdempotencyKey, v.AgentHubTicket, v.Actor, v.ActorName, v.Note)
	if err != nil {
		return PowerReview{}, false, err
	}
	if tag.RowsAffected() == 1 {
		if _, err := tx.Exec(ctx, `INSERT INTO shippin_infra_power_review_events(id,review_id,state,actor) VALUES($1,$2,'preparing',$3)`, uuid.NewString(), v.ID, v.Actor); err != nil {
			return PowerReview{}, false, err
		}
	}
	got, err := scanPowerReview(tx.QueryRow(ctx, `SELECT `+powerReviewColumns+` FROM shippin_infra_power_reviews WHERE server_id=$1 AND idempotency_key=$2`, v.ServerID, v.IdempotencyKey))
	if err != nil {
		return PowerReview{}, false, err
	}
	if got.ActionSHA256 != v.ActionSHA256 || got.Actor != v.Actor {
		return PowerReview{}, false, ErrPowerReviewConflict
	}
	if err := tx.Commit(ctx); err != nil {
		return PowerReview{}, false, err
	}
	return got, tag.RowsAffected() == 1, nil
}

func (s *Store) GetPowerReview(ctx context.Context, id, serverID string) (PowerReview, error) {
	if uuid.Validate(id) != nil || uuid.Validate(serverID) != nil {
		return PowerReview{}, ErrNotFound
	}
	return scanPowerReview(s.Pool.QueryRow(ctx, `SELECT `+powerReviewColumns+` FROM shippin_infra_power_reviews WHERE id=$1 AND server_id=$2`, id, serverID))
}

func (s *Store) SetPowerReviewState(ctx context.Context, id, state, ticketSHA, failure string) (PowerReview, error) {
	var sha, code any
	if ticketSHA != "" {
		sha = ticketSHA
	}
	if failure != "" {
		code = failure
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return PowerReview{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var serverID, actor string
	err = tx.QueryRow(ctx, `UPDATE shippin_infra_power_reviews SET state=$2,ticket_sha256=$3,failure_code=$4,updated_at=now()
		WHERE id=$1 AND state IN ('preparing','failed','pending_review') RETURNING server_id::text,actor`, id, state, sha, code).Scan(&serverID, &actor)
	if errors.Is(err, pgx.ErrNoRows) {
		return PowerReview{}, ErrNotFound
	}
	if err != nil {
		return PowerReview{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO shippin_infra_power_review_events(id,review_id,state,failure_code,actor) VALUES($1,$2,$3,$4,$5)`, uuid.NewString(), id, state, code, actor); err != nil {
		return PowerReview{}, err
	}
	got, err := scanPowerReview(tx.QueryRow(ctx, `SELECT `+powerReviewColumns+` FROM shippin_infra_power_reviews WHERE id=$1`, id))
	if err != nil {
		return PowerReview{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return PowerReview{}, err
	}
	return got, nil
}

// ClaimPowerReview consumes an AgentHub approval exactly once before any provider
// call. An interrupted executing state is later recovered as unknown.
func (s *Store) ClaimPowerReview(ctx context.Context, id, serverID, actionSHA, ticketSHA string, provenance any) (bool, error) {
	data, err := json.Marshal(provenance)
	if err != nil {
		return false, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var actor string
	err = tx.QueryRow(ctx, `UPDATE shippin_infra_power_reviews SET state='executing',provenance=$5,updated_at=now()
		WHERE id=$1 AND server_id=$2 AND action_sha256=$3 AND ticket_sha256=$4 AND state='pending_review' RETURNING actor`, id, serverID, actionSHA, ticketSHA, data).Scan(&actor)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO shippin_infra_power_review_events(id,review_id,state,provenance,actor) VALUES($1,$2,'executing',$3,$4)`, uuid.NewString(), id, data, actor); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func (s *Store) FinishPowerReview(ctx context.Context, id, state, failure string) error {
	var code any
	if failure != "" {
		code = failure
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var actor string
	err = tx.QueryRow(ctx, `UPDATE shippin_infra_power_reviews SET state=$2,failure_code=$3,updated_at=now() WHERE id=$1 AND state='executing' RETURNING actor`, id, state, code).Scan(&actor)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO shippin_infra_power_review_events(id,review_id,state,failure_code,actor) VALUES($1,$2,$3,$4,$5)`, uuid.NewString(), id, state, code, actor); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) RecoverPowerReviews(ctx context.Context) (int64, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `UPDATE shippin_infra_power_reviews SET state='unknown',failure_code='execution_interrupted',updated_at=now() WHERE state='executing' RETURNING id,actor`)
	if err != nil {
		return 0, err
	}
	type recovered struct{ id, actor string }
	items := []recovered{}
	for rows.Next() {
		var item recovered
		if err := rows.Scan(&item.id, &item.actor); err != nil {
			rows.Close()
			return 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	for _, item := range items {
		if _, err := tx.Exec(ctx, `INSERT INTO shippin_infra_power_review_events(id,review_id,state,failure_code,actor) VALUES($1,$2,'unknown','execution_interrupted',$3)`, uuid.NewString(), item.id, item.actor); err != nil {
			return 0, err
		}
	}
	return int64(len(items)), tx.Commit(ctx)
}
