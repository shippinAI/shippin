package servers

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrCommandConflict = errors.New("server command conflicts with its stored identity or result")
var commandSHA256 = regexp.MustCompile(`^[a-f0-9]{64}$`)

func validUUID(value string) bool   { return uuid.Validate(value) == nil }
func validSHA256(value string) bool { return commandSHA256.MatchString(value) }
func sameJSON(left, right []byte) bool {
	var a, b any
	if json.Unmarshal(left, &a) != nil || json.Unmarshal(right, &b) != nil {
		return false
	}
	canonicalA, errA := json.Marshal(a)
	canonicalB, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(canonicalA) == string(canonicalB)
}

type Command struct {
	ID             string          `json:"id"`
	Type           string          `json:"type"`
	DeploymentID   string          `json:"deployment_id"`
	Generation     int64           `json:"generation"`
	PlanSHA256     string          `json:"plan_sha256"`
	IdempotencyKey string          `json:"idempotency_key"`
	Payload        json.RawMessage `json:"payload"`
	ExpiresAt      time.Time       `json:"expires_at"`
	State          string          `json:"-"`
}

type CompletedCommand struct {
	TenantID     string
	DeploymentID string
	Generation   int64
	Result       json.RawMessage
}

// QueueProbeCommand persists an immutable, generation-bound host probe command.
func (s *Store) QueueProbeCommand(ctx context.Context, serverID, tenantID, deploymentID string,
	generation int64, planSHA256, idempotencyKey string, payload json.RawMessage, ttl time.Duration) (*Command, error) {
	if !validUUID(serverID) || !validUUID(tenantID) || !validUUID(deploymentID) || generation < 1 ||
		!validSHA256(planSHA256) || !validSHA256(idempotencyKey) || len(payload) == 0 || len(payload) > 8192 ||
		ttl < time.Second || ttl > 15*time.Minute || !json.Valid(payload) {
		return nil, errors.New("server probe command is invalid")
	}
	var existing Command
	err := s.Pool.QueryRow(ctx, `INSERT INTO shippin_server_commands
		(id,server_id,tenant_id,deployment_id,generation,plan_sha256,idempotency_key,command_type,payload,expires_at)
		SELECT $1,$2,$3,$4,$5,$6,$7,'probe_incus_application',$8,now()+($9 * interval '1 second')
		FROM shippin_deployments d JOIN servers s ON s.id=d.target_system_id
		WHERE d.id=$4 AND d.tenant_id=$3 AND d.target_system_id=$2 AND d.desired_generation=$5
		AND s.state='active' AND s.cred_expires_at>now()
		ON CONFLICT(tenant_id,idempotency_key) DO NOTHING
		RETURNING id::text,deployment_id::text,generation,plan_sha256,idempotency_key,command_type,payload,expires_at,state`,
		uuid.NewString(), serverID, tenantID, deploymentID, generation, planSHA256, idempotencyKey,
		[]byte(payload), ttl.Seconds()).Scan(&existing.ID, &existing.DeploymentID, &existing.Generation,
		&existing.PlanSHA256, &existing.IdempotencyKey, &existing.Type, &existing.Payload, &existing.ExpiresAt, &existing.State)
	if errors.Is(err, pgx.ErrNoRows) {
		err = s.Pool.QueryRow(ctx, `SELECT id::text,deployment_id::text,generation,plan_sha256,idempotency_key,
			command_type,payload,expires_at,state FROM shippin_server_commands WHERE tenant_id=$1 AND idempotency_key=$2`,
			tenantID, idempotencyKey).Scan(&existing.ID, &existing.DeploymentID, &existing.Generation,
			&existing.PlanSHA256, &existing.IdempotencyKey, &existing.Type, &existing.Payload, &existing.ExpiresAt, &existing.State)
	}
	if err != nil {
		return nil, err
	}
	var storedServer string
	if err := s.Pool.QueryRow(ctx, `SELECT server_id::text FROM shippin_server_commands WHERE id=$1`, existing.ID).Scan(&storedServer); err != nil {
		return nil, err
	}
	if storedServer != serverID || existing.DeploymentID != deploymentID || existing.Generation != generation ||
		existing.PlanSHA256 != planSHA256 || existing.IdempotencyKey != idempotencyKey || existing.Type != "probe_incus_application" ||
		!sameJSON(existing.Payload, payload) {
		return nil, ErrCommandConflict
	}
	if existing.State == "queued" && !existing.ExpiresAt.After(s.now()) {
		if err := s.Pool.QueryRow(ctx, `UPDATE shippin_server_commands SET expires_at=now()+($2 * interval '1 second')
			WHERE id=$1 AND state='queued' RETURNING expires_at`, existing.ID, ttl.Seconds()).Scan(&existing.ExpiresAt); err != nil {
			return nil, err
		}
	}
	existing.Payload = append(json.RawMessage(nil), existing.Payload...)
	return &existing, nil
}

// PendingCommand returns the oldest command for this authenticated host and
// current deployment generation. It renews an expired pending row on heartbeat
// and leaves it queued until the host submits an idempotent result.
func (s *Store) PendingCommand(ctx context.Context, serverID, credential string) (*Command, error) {
	if !validUUID(serverID) || !wellFormed(credential, CredPrefix) {
		return nil, ErrBadCred
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var authenticated string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM servers WHERE id=$1 AND cred_sha256=$2 AND state='active'
		AND cred_expires_at>now() FOR SHARE`, serverID, Hash(credential)).Scan(&authenticated); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrBadCred
		}
		return nil, err
	}
	var command Command
	err = tx.QueryRow(ctx, `UPDATE shippin_server_commands c SET expires_at=now()+interval '5 minutes'
		WHERE c.id=(SELECT p.id FROM shippin_server_commands p JOIN shippin_deployments d
			ON d.id=p.deployment_id AND d.tenant_id=p.tenant_id AND d.target_system_id=p.server_id
			WHERE p.server_id=$1 AND p.state='queued' AND p.expires_at<=now() AND d.desired_generation=p.generation
			ORDER BY p.created_at,p.id LIMIT 1 FOR UPDATE OF p)
		RETURNING c.id::text,c.deployment_id::text,c.generation,c.plan_sha256,c.idempotency_key,
			c.command_type,c.payload,c.expires_at`, serverID).
		Scan(&command.ID, &command.DeploymentID, &command.Generation, &command.PlanSHA256,
			&command.IdempotencyKey, &command.Type, &command.Payload, &command.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `SELECT c.id::text,c.deployment_id::text,c.generation,c.plan_sha256,c.idempotency_key,
		c.command_type,c.payload,c.expires_at FROM shippin_server_commands c JOIN shippin_deployments d
			ON d.id=c.deployment_id AND d.tenant_id=c.tenant_id AND d.target_system_id=c.server_id
		WHERE c.server_id=$1 AND c.state='queued' AND c.expires_at>now() AND d.desired_generation=c.generation
		ORDER BY c.created_at,c.id LIMIT 1`, serverID).
			Scan(&command.ID, &command.DeploymentID, &command.Generation, &command.PlanSHA256,
				&command.IdempotencyKey, &command.Type, &command.Payload, &command.ExpiresAt)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, tx.Commit(ctx)
	}
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &command, nil
}

// CompleteCommand authenticates the host credential and stores one bounded,
// idempotent result for a command assigned to that same server.
func (s *Store) CompleteCommand(ctx context.Context, credential, commandID string, result json.RawMessage) (*CompletedCommand, error) {
	if !wellFormed(credential, CredPrefix) || !validUUID(commandID) || len(result) == 0 || len(result) > 2048 || !json.Valid(result) {
		return nil, ErrBadCred
	}
	var command CompletedCommand
	err := s.Pool.QueryRow(ctx, `UPDATE shippin_server_commands c SET state='completed',result=$3,completed_at=now()
		FROM servers s WHERE c.id=$1 AND c.server_id=s.id AND s.cred_sha256=$2 AND s.state='active'
		AND s.cred_expires_at>now() AND c.state='queued' AND c.expires_at>now()
		RETURNING c.tenant_id::text,c.deployment_id::text,c.generation,c.result`, commandID, Hash(credential), []byte(result)).
		Scan(&command.TenantID, &command.DeploymentID, &command.Generation, &command.Result)
	if err == nil {
		return &command, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	var prior []byte
	err = s.Pool.QueryRow(ctx, `SELECT c.tenant_id::text,c.deployment_id::text,c.generation,c.result FROM shippin_server_commands c JOIN servers s ON s.id=c.server_id
		WHERE c.id=$1 AND s.cred_sha256=$2 AND s.state='active' AND s.cred_expires_at>now()
		AND c.state='completed'`, commandID, Hash(credential)).Scan(&command.TenantID, &command.DeploymentID, &command.Generation, &prior)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBadCred
	}
	if err != nil {
		return nil, err
	}
	if !sameJSON(prior, result) {
		return nil, ErrCommandConflict
	}
	command.Result = json.RawMessage(prior)
	return &command, nil
}

// ProbeCommandResult reads only a completed, matching command result.
func (s *Store) ProbeCommandResult(ctx context.Context, tenantID, idempotencyKey string) (json.RawMessage, bool, error) {
	if !validUUID(tenantID) || !validSHA256(idempotencyKey) {
		return nil, false, errors.New("probe command lookup is invalid")
	}
	var result []byte
	err := s.Pool.QueryRow(ctx, `SELECT result FROM shippin_server_commands WHERE tenant_id=$1 AND idempotency_key=$2
		AND state='completed'`, tenantID, idempotencyKey).Scan(&result)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return json.RawMessage(result), true, nil
}
