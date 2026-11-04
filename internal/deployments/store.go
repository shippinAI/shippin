package deployments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"shippin.cloud/console/internal/catalog"
)

var (
	ErrNotFound              = errors.New("deployment not found")
	ErrDefinitionUnavailable = errors.New("service definition is not deployable")
	ErrTargetUnavailable     = errors.New("target system is not available for this tenant/provider")
	ErrIdempotencyConflict   = errors.New("idempotency key was already used for a different request")
	ErrGenerationConflict    = errors.New("deployment generation changed; reload before retrying")
	ErrInvalidRequest        = errors.New("deployment request is invalid")
	ErrInvalidConfig         = errors.New("deployment configuration is invalid")
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

type CreateInput struct {
	ServiceID      string          `json:"service_id"`
	TargetSystemID string          `json:"target_system_id"`
	DesiredState   DesiredState    `json:"desired_state"`
	IdempotencyKey string          `json:"-"`
	Config         json.RawMessage `json:"config"`
}

type UpdateInput struct {
	DesiredState   DesiredState    `json:"desired_state"`
	IdempotencyKey string          `json:"-"`
	Config         json.RawMessage `json:"config,omitempty"`
}

type SourceView struct {
	Kind        string `json:"kind"`
	Reference   string `json:"reference"`
	Commit      string `json:"commit,omitempty"`
	ContextPath string `json:"context_path,omitempty"`
}

type Target struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Site     string   `json:"site"`
	Provider Provider `json:"provider"`
	State    string   `json:"state"`
}

type Deployment struct {
	ID                 string          `json:"id"`
	ServiceID          string          `json:"service_id"`
	TargetSystemID     string          `json:"target_system_id"`
	TargetProvider     Provider        `json:"target_provider"`
	Generation         int64           `json:"generation"`
	DesiredState       DesiredState    `json:"desired_state"`
	ObservedState      ObservedState   `json:"observed_state"`
	DefinitionSHA256   string          `json:"definition_sha256"`
	ConfigSchema       json.RawMessage `json:"config_schema"`
	Source             SourceView      `json:"source"`
	Config             json.RawMessage `json:"config"`
	CreatedBy          string          `json:"created_by"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedBy          string          `json:"updated_by"`
	UpdatedAt          time.Time       `json:"updated_at"`
	definitionSnapshot json.RawMessage
}

const deploymentSelect = `SELECT d.id::text, d.service_id, d.target_system_id::text,
  d.target_provider, d.desired_generation, g.desired_state, d.observed_state,
  g.definition_sha256, g.definition_snapshot::text, g.config::text,
  d.created_by, d.created_at, d.updated_by, d.updated_at
  FROM shippin_deployments d
  JOIN shippin_deployment_generations g ON g.deployment_id=d.id AND g.tenant_id=d.tenant_id
    AND g.generation=d.desired_generation`

const deploymentGenerationSelect = `SELECT d.id::text, d.service_id, d.target_system_id::text,
  d.target_provider, g.generation, g.desired_state, d.observed_state,
  g.definition_sha256, g.definition_snapshot::text, g.config::text,
  d.created_by, d.created_at, d.updated_by, d.updated_at
  FROM shippin_deployments d
  JOIN shippin_deployment_generations g ON g.deployment_id=d.id AND g.tenant_id=d.tenant_id`

func scanDeployment(row pgx.Row) (*Deployment, error) {
	d := &Deployment{}
	var provider, desired, observed string
	var snapshot, config []byte
	if err := row.Scan(&d.ID, &d.ServiceID, &d.TargetSystemID, &provider, &d.Generation, &desired,
		&observed, &d.DefinitionSHA256, &snapshot, &config, &d.CreatedBy, &d.CreatedAt, &d.UpdatedBy, &d.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	d.TargetProvider, d.DesiredState, d.ObservedState = Provider(provider), DesiredState(desired), ObservedState(observed)
	d.Config = json.RawMessage(config)
	d.definitionSnapshot = json.RawMessage(snapshot)
	var definition DefinitionSnapshot
	if err := json.Unmarshal(snapshot, &definition); err != nil {
		return nil, fmt.Errorf("stored deployment definition snapshot is invalid: %w", err)
	}
	d.ConfigSchema = catalog.RedactSecretReferences(definition.ConfigSchema)
	d.Source = SourceView{Kind: definition.Source.Kind, Reference: definition.Source.Reference,
		Commit: definition.Source.Commit, ContextPath: definition.Source.ContextPath}
	return d, nil
}

// Create records an initial desired generation only. It does not enqueue work or call a provider.
// A future reconciler must obtain the configured AgentHub decision before any provider mutation.
func (s *Store) Create(ctx context.Context, tenant, actor string, input CreateInput) (*Deployment, bool, error) {
	if !validUUID(tenant) || !validUUID(input.TargetSystemID) || !validUUID(input.IdempotencyKey) ||
		!serviceIDPattern.MatchString(input.ServiceID) || input.DesiredState != DesiredPresent {
		return nil, false, ErrInvalidRequest
	}
	if len(actor) == 0 || len(actor) > 255 || strings.TrimSpace(actor) != actor || strings.ContainsAny(actor, "\r\n\x00") {
		return nil, false, errors.New("deployment actor is invalid")
	}
	bodyHash, err := fingerprintCreateInput(tenant, actor, input)
	if err != nil {
		return nil, false, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	lockKey := tenant + ":" + input.IdempotencyKey
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return nil, false, err
	}
	var priorID string
	var priorGeneration int64
	var priorHash string
	err = tx.QueryRow(ctx, `SELECT deployment_id::text, generation, request_sha256
		FROM shippin_deployment_generations WHERE tenant_id=$1 AND idempotency_key=$2`, tenant, input.IdempotencyKey).
		Scan(&priorID, &priorGeneration, &priorHash)
	if err == nil {
		if priorHash != bodyHash {
			return nil, false, ErrIdempotencyConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, false, err
		}
		deployment, err := s.getGeneration(ctx, tenant, priorID, priorGeneration)
		return deployment, false, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, err
	}
	definition, err := loadDefinition(ctx, tx, input.ServiceID)
	if err != nil {
		return nil, false, err
	}
	var provider string
	err = tx.QueryRow(ctx, `SELECT s.hv_kind FROM servers s
		JOIN server_assignments a ON a.server_id=s.id AND a.tenant_id=$2
		WHERE s.id=$1 AND s.state IN ('planned','active') AND s.hv_kind IN ('incus','proxmox')
		FOR SHARE OF s, a`, input.TargetSystemID, tenant).Scan(&provider)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, ErrTargetUnavailable
	}
	if err != nil {
		return nil, false, err
	}
	request := DeploymentRequest{TenantID: tenant, ServiceID: input.ServiceID, TargetSystemID: input.TargetSystemID,
		TargetProvider: Provider(provider), DesiredState: input.DesiredState, IdempotencyKey: input.IdempotencyKey,
		Actor: actor, Definition: definition, Config: input.Config}
	if err := request.Validate(); err != nil {
		return nil, false, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	definitionHash, err := definition.Fingerprint()
	if err != nil {
		return nil, false, err
	}
	snapshot, err := json.Marshal(definition)
	if err != nil {
		return nil, false, err
	}
	deploymentID, err := uuid.NewV7()
	if err != nil {
		return nil, false, err
	}
	eventID, err := uuid.NewV7()
	if err != nil {
		return nil, false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO shippin_deployments
		(id,tenant_id,service_id,target_system_id,target_provider,desired_generation,observed_state,created_by,updated_by)
		VALUES($1,$2,$3,$4,$5,1,'unknown',$6,$6)`, deploymentID, tenant, input.ServiceID,
		input.TargetSystemID, provider, actor); err != nil {
		return nil, false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO shippin_deployment_generations
		(deployment_id,tenant_id,generation,desired_state,definition_sha256,definition_snapshot,config,idempotency_key,request_sha256,created_by)
		VALUES($1,$2,1,$3,$4,$5,$6,$7,$8,$9)`, deploymentID, tenant, input.DesiredState,
		definitionHash, snapshot, []byte(input.Config), input.IdempotencyKey, bodyHash, actor); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, false, ErrIdempotencyConflict
		}
		return nil, false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO shippin_deployment_events
		(id,deployment_id,tenant_id,generation,event_type,actor) VALUES($1,$2,$3,1,$4,$5)`,
		eventID, deploymentID, tenant, EventRequested, actor); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, err
	}
	deployment, err := s.Get(ctx, tenant, deploymentID.String())
	return deployment, true, err
}

// Update appends a desired generation when the supplied base generation still matches.
// It records intent only and never calls a provider.
func (s *Store) Update(ctx context.Context, tenant, id, actor string, expectedGeneration int64, input UpdateInput) (*Deployment, bool, error) {
	if !validUUID(tenant) || !validUUID(id) || !validUUID(input.IdempotencyKey) || expectedGeneration < 1 ||
		(input.DesiredState != DesiredPresent && input.DesiredState != DesiredAbsent) || len(actor) == 0 || len(actor) > 255 ||
		strings.TrimSpace(actor) != actor || strings.ContainsAny(actor, "\r\n\x00") {
		return nil, false, ErrInvalidRequest
	}
	bodyHash, err := fingerprintUpdateInput(tenant, id, actor, expectedGeneration, input)
	if err != nil {
		return nil, false, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, tenant+":"+input.IdempotencyKey); err != nil {
		return nil, false, err
	}
	var priorID, priorHash string
	var priorGeneration int64
	err = tx.QueryRow(ctx, `SELECT deployment_id::text,generation,request_sha256 FROM shippin_deployment_generations
		WHERE tenant_id=$1 AND idempotency_key=$2`, tenant, input.IdempotencyKey).Scan(&priorID, &priorGeneration, &priorHash)
	if err == nil {
		if priorHash != bodyHash {
			return nil, false, ErrIdempotencyConflict
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, false, err
		}
		deployment, err := s.getGeneration(ctx, tenant, priorID, priorGeneration)
		return deployment, false, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, false, err
	}
	current, err := scanDeployment(tx.QueryRow(ctx, deploymentSelect+` WHERE d.tenant_id=$1 AND d.id=$2 FOR UPDATE OF d,g`, tenant, id))
	if err != nil {
		return nil, false, err
	}
	if current.Generation != expectedGeneration {
		return nil, false, ErrGenerationConflict
	}
	var definition DefinitionSnapshot
	if err := json.Unmarshal(current.definitionSnapshot, &definition); err != nil {
		return nil, false, ErrDefinitionUnavailable
	}
	if len(input.Config) == 0 {
		input.Config = current.Config
	}
	request := DeploymentRequest{TenantID: tenant, ServiceID: current.ServiceID, TargetSystemID: current.TargetSystemID,
		TargetProvider: current.TargetProvider, DesiredState: input.DesiredState, IdempotencyKey: input.IdempotencyKey,
		Actor: actor, Definition: definition, Config: input.Config}
	if err := request.Validate(); err != nil {
		return nil, false, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	nextGeneration := expectedGeneration + 1
	eventID, err := uuid.NewV7()
	if err != nil {
		return nil, false, err
	}
	if _, err := tx.Exec(ctx, `UPDATE shippin_deployments SET desired_generation=$3,observed_state='pending',updated_by=$4,updated_at=now()
		WHERE tenant_id=$1 AND id=$2 AND desired_generation=$5`, tenant, id, nextGeneration, actor, expectedGeneration); err != nil {
		return nil, false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO shippin_deployment_generations
		(deployment_id,tenant_id,generation,desired_state,definition_sha256,definition_snapshot,config,idempotency_key,request_sha256,created_by)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, tenant, nextGeneration, input.DesiredState,
		current.DefinitionSHA256, current.definitionSnapshot, []byte(input.Config), input.IdempotencyKey, bodyHash, actor); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, false, ErrIdempotencyConflict
		}
		return nil, false, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO shippin_deployment_events
		(id,deployment_id,tenant_id,generation,event_type,actor) VALUES($1,$2,$3,$4,$5,$6)`,
		eventID, id, tenant, nextGeneration, EventRequested, actor); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, err
	}
	deployment, err := s.Get(ctx, tenant, id)
	return deployment, true, err
}

func fingerprintUpdateInput(tenant, id, actor string, expected int64, input UpdateInput) (string, error) {
	return hashCanonical(struct {
		TenantID string          `json:"tenant_id"`
		ID       string          `json:"deployment_id"`
		Actor    string          `json:"actor"`
		Expected int64           `json:"expected_generation"`
		Desired  DesiredState    `json:"desired_state"`
		Key      string          `json:"idempotency_key"`
		Config   json.RawMessage `json:"config"`
	}{tenant, id, actor, expected, input.DesiredState, input.IdempotencyKey, input.Config})
}

// fingerprintCreateInput excludes the catalog snapshot because replaying an identical HTTP body
// must return the original generation even if an operator later edits the catalog definition.
func fingerprintCreateInput(tenant, actor string, input CreateInput) (string, error) {
	return (&DeploymentRequest{TenantID: tenant, ServiceID: input.ServiceID, TargetSystemID: input.TargetSystemID,
		DesiredState: input.DesiredState, IdempotencyKey: input.IdempotencyKey, Actor: actor,
		Config: input.Config}).RequestFingerprint()
}

func loadDefinition(ctx context.Context, tx pgx.Tx, serviceID string) (DefinitionSnapshot, error) {
	var sourceRaw, schemaRaw, defaultsRaw []byte
	err := tx.QueryRow(ctx, `SELECT deployment_source::text, config_schema::text, default_config::text
		FROM catalog_services WHERE id=$1 AND active=true AND deployment_source IS NOT NULL FOR SHARE`, serviceID).
		Scan(&sourceRaw, &schemaRaw, &defaultsRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return DefinitionSnapshot{}, ErrDefinitionUnavailable
	}
	if err != nil {
		return DefinitionSnapshot{}, err
	}
	definition := DefinitionSnapshot{ServiceID: serviceID, ConfigSchema: schemaRaw, DefaultConfig: defaultsRaw}
	if err := json.Unmarshal(sourceRaw, &definition.Source); err != nil {
		return DefinitionSnapshot{}, ErrDefinitionUnavailable
	}
	return definition, nil
}

func (s *Store) Get(ctx context.Context, tenant, id string) (*Deployment, error) {
	if !validUUID(tenant) || !validUUID(id) {
		return nil, ErrNotFound
	}
	return scanDeployment(s.pool.QueryRow(ctx, deploymentSelect+` WHERE d.tenant_id=$1 AND d.id=$2`, tenant, id))
}

func (s *Store) Targets(ctx context.Context, tenant string) ([]Target, error) {
	if !validUUID(tenant) {
		return nil, ErrNotFound
	}
	rows, err := s.pool.Query(ctx, `SELECT s.id::text,s.label,s.site,s.hv_kind,s.state FROM servers s
		JOIN server_assignments a ON a.server_id=s.id AND a.tenant_id=$1
		WHERE s.state IN ('planned','active') AND s.hv_kind IN ('incus','proxmox') ORDER BY s.label,s.id`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	targets := make([]Target, 0)
	for rows.Next() {
		var target Target
		var provider string
		if err := rows.Scan(&target.ID, &target.Label, &target.Site, &provider, &target.State); err != nil {
			return nil, err
		}
		target.Provider = Provider(provider)
		targets = append(targets, target)
	}
	return targets, rows.Err()
}

func (s *Store) getGeneration(ctx context.Context, tenant, id string, generation int64) (*Deployment, error) {
	if !validUUID(tenant) || !validUUID(id) || generation < 1 {
		return nil, ErrNotFound
	}
	return scanDeployment(s.pool.QueryRow(ctx, deploymentGenerationSelect+
		` WHERE d.tenant_id=$1 AND d.id=$2 AND g.generation=$3`, tenant, id, generation))
}

func (s *Store) List(ctx context.Context, tenant string, limit int) ([]Deployment, error) {
	if !validUUID(tenant) {
		return nil, ErrNotFound
	}
	if limit < 1 || limit > 200 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, deploymentSelect+` WHERE d.tenant_id=$1 ORDER BY d.updated_at DESC,d.id LIMIT $2`, tenant, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	deployments := make([]Deployment, 0)
	for rows.Next() {
		deployment, err := scanDeployment(rows)
		if err != nil {
			return nil, err
		}
		deployments = append(deployments, *deployment)
	}
	return deployments, rows.Err()
}

func (s *Store) Events(ctx context.Context, tenant, id string, limit int) ([]DeploymentEvent, error) {
	if _, err := s.Get(ctx, tenant, id); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `SELECT id::text,deployment_id::text,tenant_id::text,generation,event_type,
		coalesce(observed_state,''),coalesce(provider_operation_id,''),coalesce(health_status,''),health_duration_ms,
		coalesce(failure_code,''),actor,recorded_at FROM shippin_deployment_events
		WHERE tenant_id=$1 AND deployment_id=$2 ORDER BY recorded_at,id LIMIT $3`, tenant, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := make([]DeploymentEvent, 0)
	for rows.Next() {
		var event DeploymentEvent
		var eventType, observed, durationStatus string
		if err := rows.Scan(&event.ID, &event.DeploymentID, &event.TenantID, &event.Generation, &eventType,
			&observed, &event.ProviderOperationID, &durationStatus, &event.HealthDurationMS, &event.FailureCode, &event.Actor, &event.RecordedAt); err != nil {
			return nil, err
		}
		event.Type, event.ObservedState, event.HealthStatus = EventType(eventType), ObservedState(observed), durationStatus
		events = append(events, event)
	}
	return events, rows.Err()
}
