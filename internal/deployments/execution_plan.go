package deployments

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
)

// ExecutionPlanVersion versions Shippin's reviewed provider-step contract. It
// is independent of a catalog runtime recipe version.
const ExecutionPlanVersion = 1

// ExecutionPlan is a deterministic description of one desired deployment
// generation. It contains hashes instead of configuration values so secret
// references and resolved secret bytes are not copied into this record.
// Having a plan does not mean an executor for it is installed.
type ExecutionPlan struct {
	Version            int             `json:"version"`
	TenantID           string          `json:"tenant_id"`
	DeploymentID       string          `json:"deployment_id"`
	Generation         int64           `json:"generation"`
	DesiredState       DesiredState    `json:"desired_state"`
	Provider           Provider        `json:"provider"`
	TargetSystemID     string          `json:"target_system_id"`
	ServiceID          string          `json:"service_id"`
	DefinitionSHA256   string          `json:"definition_sha256"`
	SourceSHA256       string          `json:"source_sha256"`
	ConfigSHA256       string          `json:"config_sha256"`
	TargetConfigSHA256 string          `json:"target_config_sha256"`
	RuntimeKind        string          `json:"runtime_kind"`
	RuntimeVersion     int             `json:"runtime_version"`
	Steps              []ExecutionStep `json:"steps"`
	UnsupportedReason  string          `json:"unsupported_reason,omitempty"`
}

// ExecutionStep names a bounded provider operation in the approved plan.
// IdempotencyKey is deterministic for a tenant, deployment generation and step.
type ExecutionStep struct {
	ID             string `json:"id"`
	Operation      string `json:"operation"`
	IdempotencyKey string `json:"idempotency_key"`
}

// BuildExecutionPlan derives the plan only from a stored immutable generation
// snapshot. It performs no provider or secret-manager I/O.
func BuildExecutionPlan(tenantID string, deployment *Deployment) (ExecutionPlan, error) {
	if !validUUID(tenantID) || deployment == nil || !validUUID(deployment.ID) ||
		!validUUID(deployment.TargetSystemID) || deployment.Generation < 1 ||
		(deployment.DesiredState != DesiredPresent && deployment.DesiredState != DesiredAbsent) {
		return ExecutionPlan{}, ErrInvalidRequest
	}
	var definition DefinitionSnapshot
	if len(deployment.definitionSnapshot) == 0 || json.Unmarshal(deployment.definitionSnapshot, &definition) != nil {
		return ExecutionPlan{}, ErrDefinitionUnavailable
	}
	definitionHash, err := definition.Fingerprint()
	if err != nil || definitionHash != deployment.DefinitionSHA256 || definition.ServiceID != deployment.ServiceID {
		return ExecutionPlan{}, ErrDefinitionUnavailable
	}
	if err := definition.Validate(deployment.Config); err != nil {
		return ExecutionPlan{}, ErrDefinitionUnavailable
	}
	targetConfig, err := json.Marshal(deployment.TargetConfig)
	if err != nil || validateTargetConfig(targetConfig, deployment.TargetProvider, definition.Runtime) != nil {
		return ExecutionPlan{}, ErrInvalidConfig
	}
	sourceHash, err := hashCanonical(definition.Source)
	if err != nil {
		return ExecutionPlan{}, err
	}
	configHash, err := hashCanonical(json.RawMessage(deployment.Config))
	if err != nil {
		return ExecutionPlan{}, errors.New("deployment configuration cannot be fingerprinted")
	}
	targetHash, err := hashCanonical(json.RawMessage(targetConfig))
	if err != nil {
		return ExecutionPlan{}, errors.New("deployment target configuration cannot be fingerprinted")
	}
	plan := ExecutionPlan{
		Version: ExecutionPlanVersion, TenantID: tenantID, DeploymentID: deployment.ID,
		Generation: deployment.Generation, DesiredState: deployment.DesiredState,
		Provider: deployment.TargetProvider, TargetSystemID: deployment.TargetSystemID,
		ServiceID: deployment.ServiceID, DefinitionSHA256: definitionHash,
		SourceSHA256: sourceHash, ConfigSHA256: configHash, TargetConfigSHA256: targetHash,
		RuntimeKind: definition.Runtime.Kind, RuntimeVersion: definition.Runtime.Version,
		Steps: []ExecutionStep{},
	}
	if deployment.TargetProvider != ProviderIncus || definition.Runtime.Kind != "incus_oci_container" ||
		!definition.Runtime.SupportsProvider(string(deployment.TargetProvider)) {
		plan.UnsupportedReason = "executor_not_installed"
	} else if definition.Runtime.Version < 2 {
		plan.UnsupportedReason = "runtime_recipe_required"
	} else if deployment.DesiredState == DesiredAbsent {
		// The reviewed Incus OCI stop/remove/absence path is implemented. Data
		// deletion remains a separate operation and is never implied here.
		plan.UnsupportedReason = ""
	} else {
		plan.UnsupportedReason = "executor_not_installed"
	}
	stepIDs := []struct{ id, operation string }{}
	if deployment.DesiredState == DesiredPresent {
		if definition.Runtime.Storage != nil {
			stepIDs = append(stepIDs, struct{ id, operation string }{"storage", "ensure_persistent_volume"})
		}
		stepIDs = append(stepIDs,
			struct{ id, operation string }{"workload", "ensure_oci_instance"},
			struct{ id, operation string }{"power", "ensure_running"},
			struct{ id, operation string }{"readback", "verify_instance_identity"},
			struct{ id, operation string }{"health", "verify_declared_health"},
		)
	} else {
		stepIDs = append(stepIDs,
			struct{ id, operation string }{"power", "stop_owned_instance"},
			struct{ id, operation string }{"workload", "delete_owned_instance_preserve_volume"},
			struct{ id, operation string }{"readback", "verify_instance_absent"},
		)
	}
	for _, item := range stepIDs {
		key, err := StepIdempotencyKey(tenantID, deployment.ID, deployment.Generation, plan.Version,
			deployment.TargetProvider, deployment.DesiredState, item.id, item.operation)
		if err != nil {
			return ExecutionPlan{}, err
		}
		plan.Steps = append(plan.Steps, ExecutionStep{ID: item.id, Operation: item.operation, IdempotencyKey: key})
	}
	return plan, nil
}

// Fingerprint returns the canonical SHA-256 digest bound into the AgentHub
// action. Step ordering is significant and duplicate steps are rejected.
func (p ExecutionPlan) Fingerprint() (string, error) {
	if p.Version != ExecutionPlanVersion || !validUUID(p.TenantID) || !validUUID(p.DeploymentID) ||
		!validUUID(p.TargetSystemID) || p.Generation < 1 || p.DesiredState != DesiredPresent && p.DesiredState != DesiredAbsent ||
		(p.Provider != ProviderIncus && p.Provider != ProviderProxmox) || !serviceIDPattern.MatchString(p.ServiceID) ||
		p.RuntimeKind == "" || p.RuntimeVersion < 1 || p.RuntimeVersion > 3 ||
		!validSHA256Value(p.DefinitionSHA256) || !validSHA256Value(p.SourceSHA256) ||
		!validSHA256Value(p.ConfigSHA256) || !validSHA256Value(p.TargetConfigSHA256) {
		return "", ErrInvalidRequest
	}
	if p.UnsupportedReason != "" && p.UnsupportedReason != "runtime_recipe_required" && p.UnsupportedReason != "executor_not_installed" {
		return "", ErrInvalidRequest
	}
	seen := make(map[string]struct{}, len(p.Steps))
	for _, step := range p.Steps {
		if step.ID == "" || step.Operation == "" || !validSHA256Value(step.IdempotencyKey) {
			return "", ErrInvalidRequest
		}
		if _, ok := seen[step.ID]; ok {
			return "", ErrInvalidRequest
		}
		seen[step.ID] = struct{}{}
	}
	canonical, err := canonicalJSON(p)
	if err != nil {
		return "", errors.New("deployment execution plan is invalid JSON")
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

// StepIdempotencyKey exposes the deterministic key format as a constructor for
// executor and recovery tests without introducing a second plan authority.
func StepIdempotencyKey(tenantID, deploymentID string, generation int64, planVersion int, provider Provider,
	desiredState DesiredState, stepID, operation string) (string, error) {
	if !validUUID(tenantID) || !validUUID(deploymentID) || generation < 1 || planVersion != ExecutionPlanVersion ||
		(provider != ProviderIncus && provider != ProviderProxmox) ||
		(desiredState != DesiredPresent && desiredState != DesiredAbsent) || stepID == "" || operation == "" {
		return "", ErrInvalidRequest
	}
	return hashCanonical(struct {
		TenantID     string       `json:"tenant_id"`
		DeploymentID string       `json:"deployment_id"`
		Generation   int64        `json:"generation"`
		PlanVersion  int          `json:"plan_version"`
		Provider     Provider     `json:"provider"`
		DesiredState DesiredState `json:"desired_state"`
		StepID       string       `json:"step_id"`
		Operation    string       `json:"operation"`
	}{tenantID, deploymentID, generation, planVersion, provider, desiredState, stepID, operation})
}
