// Package deployments owns durable desired and observed deployment state.
// Provider execution belongs to the provider drivers and is gated by the configured review owner.
package deployments

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/catalog"
)

type Provider string

const (
	ProviderIncus   Provider = "incus"
	ProviderProxmox Provider = "proxmox"
)

type DesiredState string

const (
	DesiredPresent DesiredState = "present"
	DesiredAbsent  DesiredState = "absent"
)

type ObservedState string

const (
	ObservedUnknown      ObservedState = "unknown"
	ObservedPending      ObservedState = "pending"
	ObservedProvisioning ObservedState = "provisioning"
	ObservedReady        ObservedState = "ready"
	ObservedDegraded     ObservedState = "degraded"
	ObservedFailed       ObservedState = "failed"
	ObservedAbsent       ObservedState = "absent"
)

type EventType string

const (
	EventRequested          EventType = "requested"
	EventReviewAllowed      EventType = "review_allowed"
	EventReviewDenied       EventType = "review_denied"
	EventOperationStarted   EventType = "operation_started"
	EventProviderObserved   EventType = "provider_observed"
	EventHealthObserved     EventType = "health_observed"
	EventOperationFailed    EventType = "operation_failed"
	EventOperationCancelled EventType = "operation_cancelled"
)

type DefinitionSnapshot struct {
	ServiceID     string                   `json:"service_id"`
	Source        catalog.DeploymentSource `json:"source"`
	Runtime       catalog.RuntimeContract  `json:"runtime"`
	ConfigSchema  json.RawMessage          `json:"config_schema"`
	DefaultConfig json.RawMessage          `json:"default_config"`
}

func (s DefinitionSnapshot) Validate(config json.RawMessage) error {
	if !serviceIDPattern.MatchString(s.ServiceID) {
		return errors.New("service id is invalid")
	}
	if err := s.Source.Validate(); err != nil {
		return fmt.Errorf("deployment source is invalid: %w", err)
	}
	if err := s.Runtime.Validate(s.Source); err != nil {
		return fmt.Errorf("deployment runtime is invalid: %w", err)
	}
	if err := catalog.ValidateDeploymentConfig(s.ConfigSchema, s.DefaultConfig); err != nil {
		return fmt.Errorf("deployment definition is invalid: %w", err)
	}
	if err := catalog.ValidateDeploymentConfigValues(s.ConfigSchema, config); err != nil {
		return fmt.Errorf("deployment configuration is invalid: %w", err)
	}
	if err := catalog.ValidateRuntimeConfigBindings(s.Runtime, s.ConfigSchema, config); err != nil {
		return fmt.Errorf("deployment runtime configuration is invalid: %w", err)
	}
	return nil
}

// Fingerprint hashes a canonical JSON encoding of the pinned catalog revision. It contains only
// source metadata, schema and defaults; secret bytes must never be part of a definition snapshot.
func (s DefinitionSnapshot) Fingerprint() (string, error) {
	canonical, err := canonicalJSON(s)
	if err != nil {
		return "", errors.New("deployment definition snapshot is invalid JSON")
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

type DeploymentRequest struct {
	TenantID       string             `json:"tenant_id"`
	ServiceID      string             `json:"service_id"`
	TargetSystemID string             `json:"target_system_id"`
	TargetProvider Provider           `json:"target_provider"`
	DesiredState   DesiredState       `json:"desired_state"`
	IdempotencyKey string             `json:"idempotency_key"`
	Actor          string             `json:"actor"`
	Definition     DefinitionSnapshot `json:"definition"`
	Config         json.RawMessage    `json:"config"`
	TargetConfig   json.RawMessage    `json:"target_config"`
}

func (r DeploymentRequest) Validate() error {
	if !validUUID(r.TenantID) || !validUUID(r.TargetSystemID) || !validUUID(r.IdempotencyKey) {
		return errors.New("tenant, target system and idempotency key must be UUIDs")
	}
	if r.TargetProvider != ProviderIncus && r.TargetProvider != ProviderProxmox {
		return errors.New("target provider must be incus or proxmox")
	}
	if r.DesiredState != DesiredPresent && r.DesiredState != DesiredAbsent {
		return errors.New("desired state must be present or absent")
	}
	if !r.Definition.Runtime.SupportsProvider(string(r.TargetProvider)) {
		return errors.New("target provider is incompatible with the declared runtime")
	}
	if len(r.Actor) == 0 || len(r.Actor) > 255 || strings.TrimSpace(r.Actor) != r.Actor || strings.ContainsAny(r.Actor, "\r\n\x00") {
		return errors.New("actor is invalid")
	}
	if r.ServiceID != r.Definition.ServiceID {
		return errors.New("service id does not match the pinned definition")
	}
	if err := r.Definition.Validate(r.Config); err != nil {
		return err
	}
	if err := validateTargetConfig(r.TargetConfig, r.TargetProvider, r.Definition.Runtime); err != nil {
		return err
	}
	return nil
}

type TargetConfig struct {
	Storage *TargetStorageConfig `json:"storage,omitempty"`
}

type TargetStorageConfig struct {
	Pool string `json:"pool"`
}

var targetPoolPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)

// normalizeTargetConfig rejects unknown provider-specific fields, supplies the
// canonical empty object and verifies that placement settings match the pinned
// runtime. The caller stores this value in the immutable generation.
func normalizeTargetConfig(raw json.RawMessage, provider Provider, runtime catalog.RuntimeContract) (json.RawMessage, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, errors.New("target configuration must be a JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	var config TargetConfig
	if err := decoder.Decode(&config); err != nil {
		return nil, errors.New("target configuration is invalid")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("target configuration must contain one JSON object")
	}
	if config.Storage != nil && (!targetPoolPattern.MatchString(config.Storage.Pool) || strings.TrimSpace(config.Storage.Pool) != config.Storage.Pool) {
		return nil, errors.New("target storage pool is invalid")
	}
	if runtime.Storage != nil {
		if provider != ProviderIncus || config.Storage == nil || config.Storage.Pool == "" {
			return nil, errors.New("this runtime requires an explicitly selected Incus storage pool")
		}
	} else if config.Storage != nil {
		return nil, errors.New("target storage selection is not declared by this runtime")
	}
	return json.Marshal(config)
}

func validateTargetConfig(raw json.RawMessage, provider Provider, runtime catalog.RuntimeContract) error {
	_, err := normalizeTargetConfig(raw, provider, runtime)
	return err
}

// RequestFingerprint binds a tenant's idempotency key to the exact desired input.
func (r DeploymentRequest) RequestFingerprint() (string, error) {
	canonical, err := canonicalJSON(r)
	if err != nil {
		return "", errors.New("deployment request is invalid JSON")
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

type DeploymentEvent struct {
	ID                  string                    `json:"id"`
	DeploymentID        string                    `json:"deployment_id"`
	TenantID            string                    `json:"tenant_id"`
	Generation          int64                     `json:"generation"`
	Type                EventType                 `json:"event_type"`
	ObservedState       ObservedState             `json:"observed_state,omitempty"`
	ProviderOperationID string                    `json:"provider_operation_id,omitempty"`
	HealthStatus        string                    `json:"health_status,omitempty"`
	HealthDurationMS    *int64                    `json:"health_duration_ms,omitempty"`
	FailureCode         string                    `json:"failure_code,omitempty"`
	ReviewProvenance    *ReviewDecisionProvenance `json:"review_provenance,omitempty"`
	IdempotencyKey      string                    `json:"-"`
	Actor               string                    `json:"actor"`
	RecordedAt          time.Time                 `json:"recorded_at"`
}

func (e DeploymentEvent) Validate() error {
	if !validUUID(e.ID) || !validUUID(e.DeploymentID) || !validUUID(e.TenantID) || e.Generation < 1 {
		return errors.New("event identity and generation are invalid")
	}
	if _, ok := eventTypes[e.Type]; !ok {
		return errors.New("event type is invalid")
	}
	if e.ObservedState != "" && !validObservedState(e.ObservedState) {
		return errors.New("observed state is invalid")
	}
	if e.ProviderOperationID != "" && (!providerOperationIDPattern.MatchString(e.ProviderOperationID) || strings.Contains(e.ProviderOperationID, "://")) {
		return errors.New("provider operation id is invalid")
	}
	if e.HealthStatus != "" && e.HealthStatus != "unknown" && e.HealthStatus != "passing" && e.HealthStatus != "failing" {
		return errors.New("health status is invalid")
	}
	if e.HealthDurationMS != nil && (*e.HealthDurationMS < 0 || *e.HealthDurationMS > 3_600_000) {
		return errors.New("health duration is out of bounds")
	}
	if e.FailureCode != "" && !failureCodePattern.MatchString(e.FailureCode) {
		return errors.New("failure code must be a normalized code, not raw provider output")
	}
	if e.Type == EventOperationFailed && e.FailureCode == "" {
		return errors.New("failed events require a normalized failure code")
	}
	if e.Type == EventHealthObserved && e.HealthStatus == "" {
		return errors.New("health observations require a health status")
	}
	if e.Type == EventReviewAllowed && (e.ReviewProvenance == nil || e.FailureCode != "") {
		return errors.New("allowed review events require authorization provenance")
	}
	if e.Type == EventReviewDenied && (e.ReviewProvenance != nil || e.FailureCode == "") {
		return errors.New("denied review events require a normalized failure code")
	}
	if e.Type != EventReviewAllowed && e.ReviewProvenance != nil {
		return errors.New("authorization provenance is only valid on allowed review events")
	}
	if e.ReviewProvenance != nil {
		if err := e.ReviewProvenance.Validate(); err != nil {
			return err
		}
	}
	if len(e.Actor) == 0 || len(e.Actor) > 255 || strings.TrimSpace(e.Actor) != e.Actor || strings.ContainsAny(e.Actor, "\r\n\x00") {
		return errors.New("event actor is invalid")
	}
	return nil
}

var (
	serviceIDPattern           = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,39}$`)
	failureCodePattern         = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,79}$`)
	providerOperationIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,254}$`)
	eventTypes                 = map[EventType]struct{}{
		EventRequested: {}, EventReviewAllowed: {}, EventReviewDenied: {}, EventOperationStarted: {},
		EventProviderObserved: {}, EventHealthObserved: {}, EventOperationFailed: {}, EventOperationCancelled: {},
	}
)

func validUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

func validObservedState(state ObservedState) bool {
	switch state {
	case ObservedUnknown, ObservedPending, ObservedProvisioning, ObservedReady, ObservedDegraded, ObservedFailed, ObservedAbsent:
		return true
	default:
		return false
	}
}

func canonicalJSON(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var normalized any
	if err := decoder.Decode(&normalized); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, err
	}
	return json.Marshal(normalized)
}

func hashCanonical(value any) (string, error) {
	canonical, err := canonicalJSON(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}
