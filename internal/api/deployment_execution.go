package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/deployments"
	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/hv/incus"
	"shippin.cloud/console/internal/jobs"
)

type incusOCIExecutorProvider interface {
	EnsureOCIApplicationVolume(context.Context, incus.OCIApplicationSpec) error
	InspectOCIApplicationVolume(context.Context, incus.OCIApplicationSpec) (bool, error)
	CreateOCIApplicationStopped(context.Context, incus.OCIApplicationSpec) (hv.Guest, error)
	EnsureOCIApplicationRunning(context.Context, incus.OCIApplicationSpec) (hv.Guest, error)
	StopOCIApplication(context.Context, incus.OCIApplicationSpec) error
	DeleteOCIApplicationStopped(context.Context, incus.OCIApplicationSpec) error
	InspectOCIApplicationPowerState(context.Context, incus.OCIApplicationSpec) (hv.Guest, bool, error)
	InspectOCIApplicationSnapshots(context.Context, incus.OCIApplicationSpec) (bool, error)
}

// executeIncusStorageStep runs only the first typed provider write in the
// approved Incus plan. It performs target inventory checks before recording
// intent, obtains current AgentHub authorization, consumes a one-time dispatch
// claim, then verifies provider state before recording success.
func (a *API) executeIncusStorageStep(ctx context.Context, job *jobs.Job, deployment *deployments.Deployment,
	step func(string)) (map[string]any, bool, bool, error) {
	if deployment.TargetProvider != deployments.ProviderIncus || deployment.DesiredState != deployments.DesiredPresent {
		return nil, false, false, nil
	}
	plan, planDigest, unresolved, err := a.deploymentStore().UnresolvedExecutionStepsWithLease(ctx,
		job.TenantID, deployment.ID, deployment.Generation, job.ID, job.LeaseOwner)
	if err != nil {
		return nil, false, false, err
	}
	if len(unresolved) != 0 || len(plan.Steps) == 0 || plan.Steps[0].Operation != "ensure_persistent_volume" {
		return nil, false, false, nil
	}
	review, err := a.deploymentStore().GetReview(ctx, job.TenantID, deployment.ID, deployment.Generation)
	if err != nil {
		if errors.Is(err, deployments.ErrNotFound) {
			return nil, false, false, nil
		}
		return nil, false, false, err
	}
	if review.State != "pending_review" || review.ExecutionPlanVersion != plan.Version ||
		review.ExecutionPlanSHA256 != planDigest {
		return nil, false, false, nil
	}
	execution, err := a.deploymentStore().ExecutionSnapshotWithLease(ctx, job.TenantID, deployment.ID,
		deployment.Generation, job.ID, job.LeaseOwner, planDigest)
	if err != nil {
		return nil, false, false, err
	}
	executionStep := plan.Steps[0]
	spec, err := incusOCIApplicationSpec(execution)
	if err != nil {
		return nil, false, false, err
	}
	targetConfig, err := json.Marshal(execution.Deployment.TargetConfig)
	if err != nil {
		return nil, false, false, deployments.ErrInvalidConfig
	}
	if err := a.validateDeploymentTargetConfig(ctx, job.TenantID, deployment.TargetSystemID, targetConfig); err != nil {
		return nil, false, false, err
	}
	endpoint, err := a.visibleEndpoint(ctx, job.TenantID, deployment.TargetSystemID)
	if err != nil || endpoint.Kind != string(deployments.ProviderIncus) {
		return nil, false, false, deployments.ErrTargetUnavailable
	}
	var provider incusOCIExecutorProvider
	if a.openIncusOCIExecutor != nil {
		provider, err = a.openIncusOCIExecutor(endpoint)
	} else {
		provider, err = a.openIncus(endpoint)
	}
	if err != nil {
		return nil, false, false, err
	}
	if provider == nil {
		return nil, false, false, errors.New("Incus OCI executor is unavailable")
	}
	intent, started, err := a.deploymentStore().BeginExecutionStepWithLease(ctx, job.TenantID,
		deployment.ID, deployment.Generation, job.ID, job.LeaseOwner, planDigest, executionStep)
	if err != nil {
		return nil, false, false, err
	}
	if !started {
		return map[string]any{"step_id": executionStep.ID, "step_state": intent.State,
			"provider_dispatched": false}, true, false, nil
	}
	step("authorize_provider_storage_step")
	authorization, err := a.authorizeDeploymentActionWithLease(ctx, job.TenantID,
		execution.Deployment, job.CreatedBy, job.ID, job.LeaseOwner)
	if err != nil {
		return nil, true, false, err
	}
	if authorization.Status != http.StatusOK {
		failure := authorization.FailureCode
		if failure == "" {
			failure = "agenthub_review_denied"
		}
		if _, err := a.deploymentStore().RecordExecutionStepOutcomeWithLease(ctx, job.TenantID,
			deployment.ID, deployment.Generation, job.ID, job.LeaseOwner, planDigest,
			executionStep.IdempotencyKey, "failed", "", failure); err != nil {
			return nil, true, false, err
		}
		return map[string]any{"step_id": executionStep.ID, "step_state": "failed",
			"failure_code": failure, "provider_dispatched": false}, true, false, nil
	}
	step("claim_provider_storage_dispatch")
	if err := a.deploymentStore().ClaimExecutionStepDispatchWithLease(ctx, job.TenantID,
		deployment.ID, deployment.Generation, job.ID, job.LeaseOwner, planDigest, executionStep); err != nil {
		return nil, true, false, err
	}
	step("ensure_incus_persistent_volume")
	err = provider.EnsureOCIApplicationVolume(ctx, spec)
	if err != nil {
		// Preserve the dispatched intent on ambiguous provider errors so the
		// next leased attempt performs exact read-only recovery instead of retry.
		return nil, true, true, err
	}
	step("verify_incus_persistent_volume")
	exists, err := provider.InspectOCIApplicationVolume(ctx, spec)
	if err != nil {
		return nil, true, true, err
	}
	if !exists {
		return a.recordIncusStorageOutcome(ctx, job, deployment, planDigest, executionStep,
			"unknown", "provider_state_absent", true)
	}
	return a.recordIncusStorageOutcome(ctx, job, deployment, planDigest, executionStep,
		"succeeded", "", true)
}

func (a *API) recordIncusStorageOutcome(ctx context.Context, job *jobs.Job, deployment *deployments.Deployment,
	planDigest string, executionStep deployments.ExecutionStep, state, failure string, dispatched bool) (map[string]any, bool, bool, error) {
	if _, err := a.deploymentStore().RecordExecutionStepOutcomeWithLease(ctx, job.TenantID,
		deployment.ID, deployment.Generation, job.ID, job.LeaseOwner, planDigest,
		executionStep.IdempotencyKey, state, "", failure); err != nil {
		return nil, true, dispatched, err
	}
	result := map[string]any{"step_id": executionStep.ID, "step_state": state, "provider_dispatched": dispatched}
	if failure != "" {
		result["failure_code"] = failure
	}
	return result, true, dispatched, nil
}

func incusOCIApplicationSpec(snapshot deployments.ExecutionRecoverySnapshot) (incus.OCIApplicationSpec, error) {
	runtime := snapshot.Definition.Runtime
	if runtime.Kind != "incus_oci_container" || runtime.Resources == nil || runtime.Storage == nil || runtime.Network == nil {
		return incus.OCIApplicationSpec{}, deployments.ErrDefinitionUnavailable
	}
	target := snapshot.Deployment.TargetConfig
	if target.Storage == nil || target.Network == nil {
		return incus.OCIApplicationSpec{}, deployments.ErrInvalidConfig
	}
	name, err := incus.OCIApplicationName(snapshot.Deployment.ID)
	if err != nil {
		return incus.OCIApplicationSpec{}, err
	}
	return incus.OCIApplicationSpec{Name: name, DeploymentID: snapshot.Deployment.ID,
		Generation: snapshot.Deployment.Generation, ImageReference: snapshot.Definition.Source.Reference,
		Network: target.Network.Name, MemoryMiB: runtime.Resources.MemoryMiB, Cores: runtime.Resources.CPUCores,
		StoragePool: target.Storage.Pool, StorageSizeGiB: runtime.Storage.SizeGiB,
		MountPath: runtime.Storage.MountPath}, nil
}

var _ incusOCIExecutorProvider = (*incus.Client)(nil)

// executeIncusHealthStep records declared-health observation for the reviewed
// runtime after workload create and power-on have succeeded. This step performs
// provider read-back only and never dispatches a provider mutation.
func (a *API) executeIncusHealthStep(ctx context.Context, job *jobs.Job, deployment *deployments.Deployment,
	step func(string)) (map[string]any, bool, bool, error) {
	if deployment.TargetProvider != deployments.ProviderIncus || deployment.DesiredState != deployments.DesiredPresent {
		return nil, false, false, nil
	}
	plan, planDigest, unresolved, err := a.deploymentStore().UnresolvedExecutionStepsWithLease(ctx,
		job.TenantID, deployment.ID, deployment.Generation, job.ID, job.LeaseOwner)
	if err != nil {
		return nil, false, false, err
	}
	if len(unresolved) != 0 {
		return nil, false, false, nil
	}
	var executionStep deployments.ExecutionStep
	for _, candidate := range plan.Steps {
		if candidate.Operation == "verify_declared_health" {
			executionStep = candidate
			break
		}
	}
	if executionStep.ID == "" {
		return nil, false, false, nil
	}
	review, err := a.deploymentStore().GetReview(ctx, job.TenantID, deployment.ID, deployment.Generation)
	if err != nil {
		if errors.Is(err, deployments.ErrNotFound) {
			return nil, false, false, nil
		}
		return nil, false, false, err
	}
	if review.State != "pending_review" || review.ExecutionPlanVersion != plan.Version ||
		review.ExecutionPlanSHA256 != planDigest {
		return nil, false, false, nil
	}
	execution, err := a.deploymentStore().ExecutionSnapshotWithLease(ctx, job.TenantID,
		deployment.ID, deployment.Generation, job.ID, job.LeaseOwner, planDigest)
	if err != nil {
		return nil, false, false, err
	}
	spec, err := incusOCIApplicationSpec(execution)
	if err != nil {
		return nil, false, false, err
	}
	targetConfig, err := json.Marshal(execution.Deployment.TargetConfig)
	if err != nil {
		return nil, false, false, deployments.ErrInvalidConfig
	}
	if err := a.validateDeploymentTargetConfig(ctx, job.TenantID, deployment.TargetSystemID, targetConfig); err != nil {
		return nil, false, false, err
	}
	endpoint, err := a.visibleEndpoint(ctx, job.TenantID, deployment.TargetSystemID)
	if err != nil || endpoint.Kind != string(deployments.ProviderIncus) {
		return nil, false, false, deployments.ErrTargetUnavailable
	}
	var provider incusOCIExecutorProvider
	if a.openIncusOCIExecutor != nil {
		provider, err = a.openIncusOCIExecutor(endpoint)
	} else {
		provider, err = a.openIncus(endpoint)
	}
	if err != nil {
		return nil, false, false, err
	}
	if provider == nil {
		return nil, false, false, errors.New("Incus OCI executor is unavailable")
	}
	intent, started, err := a.deploymentStore().BeginExecutionStepWithLease(ctx, job.TenantID,
		deployment.ID, deployment.Generation, job.ID, job.LeaseOwner, planDigest, executionStep)
	if err != nil {
		return nil, false, false, err
	}
	if !started {
		return map[string]any{"step_id": executionStep.ID, "step_state": intent.State,
			"provider_dispatched": false, "failure_code": intent.FailureCode}, true, false, nil
	}
	step("verify_declared_incus_health")
	guest, exists, err := provider.InspectOCIApplicationPowerState(ctx, spec)
	if err != nil {
		return nil, true, false, err
	}
	// Provider power state is not application health. Keep the declared health
	// step unknown until a target-scoped probe has actually checked the
	// configured application endpoint.
	state, failure := "unknown", "application_health_probe_unavailable"
	healthStatus, observed := "unknown", deployments.ObservedUnknown
	if !exists {
		failure = "provider_state_absent"
	} else if !strings.EqualFold(guest.Status, "Running") {
		failure = "provider_state_mismatch"
		observed = deployments.ObservedDegraded
	}
	if _, err := a.deploymentStore().RecordExecutionStepOutcomeWithLease(ctx, job.TenantID,
		deployment.ID, deployment.Generation, job.ID, job.LeaseOwner, planDigest,
		executionStep.IdempotencyKey, state, "", failure); err != nil {
		return nil, true, false, err
	}
	eventKey := uuid.NewSHA1(uuid.NameSpaceURL, []byte("shippin-deployment-health:"+executionStep.IdempotencyKey)).String()
	if _, err := a.deploymentStore().RecordOperationEventWithLease(ctx, job.TenantID, deployment.ID,
		job.ID, job.LeaseOwner, deployments.DeploymentEvent{
			TenantID: job.TenantID, DeploymentID: deployment.ID, Generation: deployment.Generation,
			Type: deployments.EventHealthObserved, ObservedState: observed, HealthStatus: healthStatus,
			IdempotencyKey: eventKey, Actor: job.CreatedBy,
		}); err != nil {
		return nil, true, false, err
	}
	result := map[string]any{"step_id": executionStep.ID, "step_state": state,
		"provider_dispatched": false, "health_status": healthStatus, "application_probe_verified": false}
	if failure != "" {
		result["failure_code"] = failure
	}
	return result, true, false, nil
}

// executeIncusPowerStep starts only the exact reviewed workload after its
// stopped instance step has succeeded. Intent, lease, authorization, dispatch
// claim and provider state read-back are kept separate from workload creation.
func (a *API) executeIncusPowerStep(ctx context.Context, job *jobs.Job, deployment *deployments.Deployment,
	step func(string)) (map[string]any, bool, bool, error) {
	if deployment.TargetProvider != deployments.ProviderIncus || deployment.DesiredState != deployments.DesiredPresent {
		return nil, false, false, nil
	}
	plan, planDigest, unresolved, err := a.deploymentStore().UnresolvedExecutionStepsWithLease(ctx,
		job.TenantID, deployment.ID, deployment.Generation, job.ID, job.LeaseOwner)
	if err != nil {
		return nil, false, false, err
	}
	if len(unresolved) != 0 {
		return nil, false, false, nil
	}
	var executionStep deployments.ExecutionStep
	for _, candidate := range plan.Steps {
		if candidate.Operation == "ensure_running" {
			executionStep = candidate
			break
		}
	}
	if executionStep.ID == "" {
		return nil, false, false, nil
	}
	review, err := a.deploymentStore().GetReview(ctx, job.TenantID, deployment.ID, deployment.Generation)
	if err != nil {
		if errors.Is(err, deployments.ErrNotFound) {
			return nil, false, false, nil
		}
		return nil, false, false, err
	}
	if review.State != "pending_review" || review.ExecutionPlanVersion != plan.Version ||
		review.ExecutionPlanSHA256 != planDigest {
		return nil, false, false, nil
	}
	execution, err := a.deploymentStore().ExecutionSnapshotWithLease(ctx, job.TenantID,
		deployment.ID, deployment.Generation, job.ID, job.LeaseOwner, planDigest)
	if err != nil {
		return nil, false, false, err
	}
	spec, err := incusOCIApplicationSpec(execution)
	if err != nil {
		return nil, false, false, err
	}
	targetConfig, err := json.Marshal(execution.Deployment.TargetConfig)
	if err != nil {
		return nil, false, false, deployments.ErrInvalidConfig
	}
	if err := a.validateDeploymentTargetConfig(ctx, job.TenantID, deployment.TargetSystemID, targetConfig); err != nil {
		return nil, false, false, err
	}
	endpoint, err := a.visibleEndpoint(ctx, job.TenantID, deployment.TargetSystemID)
	if err != nil || endpoint.Kind != string(deployments.ProviderIncus) {
		return nil, false, false, deployments.ErrTargetUnavailable
	}
	var provider incusOCIExecutorProvider
	if a.openIncusOCIExecutor != nil {
		provider, err = a.openIncusOCIExecutor(endpoint)
	} else {
		provider, err = a.openIncus(endpoint)
	}
	if err != nil {
		return nil, false, false, err
	}
	if provider == nil {
		return nil, false, false, errors.New("Incus OCI executor is unavailable")
	}
	intent, started, err := a.deploymentStore().BeginExecutionStepWithLease(ctx, job.TenantID,
		deployment.ID, deployment.Generation, job.ID, job.LeaseOwner, planDigest, executionStep)
	if err != nil {
		return nil, false, false, err
	}
	if !started {
		return map[string]any{"step_id": executionStep.ID, "step_state": intent.State,
			"provider_dispatched": false}, true, false, nil
	}
	step("authorize_provider_power_step")
	authorization, err := a.authorizeDeploymentActionWithLease(ctx, job.TenantID,
		execution.Deployment, job.CreatedBy, job.ID, job.LeaseOwner)
	if err != nil {
		return nil, true, false, err
	}
	if authorization.Status != http.StatusOK {
		failure := authorization.FailureCode
		if failure == "" {
			failure = "agenthub_review_denied"
		}
		if _, err := a.deploymentStore().RecordExecutionStepOutcomeWithLease(ctx, job.TenantID,
			deployment.ID, deployment.Generation, job.ID, job.LeaseOwner, planDigest,
			executionStep.IdempotencyKey, "failed", "", failure); err != nil {
			return nil, true, false, err
		}
		return map[string]any{"step_id": executionStep.ID, "step_state": "failed",
			"failure_code": failure, "provider_dispatched": false}, true, false, nil
	}
	step("claim_provider_power_dispatch")
	if err := a.deploymentStore().ClaimExecutionStepDispatchWithLease(ctx, job.TenantID,
		deployment.ID, deployment.Generation, job.ID, job.LeaseOwner, planDigest, executionStep); err != nil {
		return nil, true, false, err
	}
	step("ensure_incus_oci_instance_running")
	guest, err := provider.EnsureOCIApplicationRunning(ctx, spec)
	if err != nil {
		return nil, true, true, err
	}
	state, failure := "succeeded", ""
	if !strings.EqualFold(guest.Status, "Running") {
		state, failure = "unknown", "provider_state_mismatch"
	}
	if _, err := a.deploymentStore().RecordExecutionStepOutcomeWithLease(ctx, job.TenantID,
		deployment.ID, deployment.Generation, job.ID, job.LeaseOwner, planDigest,
		executionStep.IdempotencyKey, state, "", failure); err != nil {
		return nil, true, true, err
	}
	result := map[string]any{"step_id": executionStep.ID, "step_state": state,
		"provider_dispatched": true}
	if failure != "" {
		result["failure_code"] = failure
	}
	return result, true, true, nil
}

// executeIncusWorkloadStep creates only the stopped OCI instance after every
// earlier plan step has succeeded. Storage is checked by the provider primitive
// but is never implicitly created here. Environment values are resolved only
// after the exact AgentHub decision and are audited by the Vaultix resolver.
func (a *API) executeIncusWorkloadStep(ctx context.Context, job *jobs.Job, deployment *deployments.Deployment,
	step func(string)) (map[string]any, bool, bool, error) {
	if deployment.TargetProvider != deployments.ProviderIncus || deployment.DesiredState != deployments.DesiredPresent {
		return nil, false, false, nil
	}
	plan, planDigest, unresolved, err := a.deploymentStore().UnresolvedExecutionStepsWithLease(ctx,
		job.TenantID, deployment.ID, deployment.Generation, job.ID, job.LeaseOwner)
	if err != nil {
		return nil, false, false, err
	}
	if len(unresolved) != 0 {
		return nil, false, false, nil
	}
	var executionStep deployments.ExecutionStep
	for _, candidate := range plan.Steps {
		if candidate.Operation == "ensure_oci_instance" {
			executionStep = candidate
			break
		}
	}
	if executionStep.ID == "" {
		return nil, false, false, nil
	}
	review, err := a.deploymentStore().GetReview(ctx, job.TenantID, deployment.ID, deployment.Generation)
	if err != nil {
		if errors.Is(err, deployments.ErrNotFound) {
			return nil, false, false, nil
		}
		return nil, false, false, err
	}
	if review.State != "pending_review" || review.ExecutionPlanVersion != plan.Version ||
		review.ExecutionPlanSHA256 != planDigest {
		return nil, false, false, nil
	}
	execution, err := a.deploymentStore().ExecutionSnapshotWithLease(ctx, job.TenantID,
		deployment.ID, deployment.Generation, job.ID, job.LeaseOwner, planDigest)
	if err != nil {
		return nil, false, false, err
	}
	spec, err := incusOCIApplicationSpec(execution)
	if err != nil {
		return nil, false, false, err
	}
	targetConfig, err := json.Marshal(execution.Deployment.TargetConfig)
	if err != nil {
		return nil, false, false, deployments.ErrInvalidConfig
	}
	if err := a.validateDeploymentTargetConfig(ctx, job.TenantID, deployment.TargetSystemID, targetConfig); err != nil {
		return nil, false, false, err
	}
	endpoint, err := a.visibleEndpoint(ctx, job.TenantID, deployment.TargetSystemID)
	if err != nil || endpoint.Kind != string(deployments.ProviderIncus) {
		return nil, false, false, deployments.ErrTargetUnavailable
	}
	var provider incusOCIExecutorProvider
	if a.openIncusOCIExecutor != nil {
		provider, err = a.openIncusOCIExecutor(endpoint)
	} else {
		provider, err = a.openIncus(endpoint)
	}
	if err != nil {
		return nil, false, false, err
	}
	if provider == nil {
		return nil, false, false, errors.New("Incus OCI executor is unavailable")
	}
	intent, started, err := a.deploymentStore().BeginExecutionStepWithLease(ctx, job.TenantID,
		deployment.ID, deployment.Generation, job.ID, job.LeaseOwner, planDigest, executionStep)
	if err != nil {
		return nil, false, false, err
	}
	if !started {
		return map[string]any{"step_id": executionStep.ID, "step_state": intent.State,
			"provider_dispatched": false}, true, false, nil
	}
	step("authorize_provider_workload_step")
	authorization, err := a.authorizeDeploymentActionWithLease(ctx, job.TenantID,
		execution.Deployment, job.CreatedBy, job.ID, job.LeaseOwner)
	if err != nil {
		return nil, true, false, err
	}
	if authorization.Status != http.StatusOK {
		failure := authorization.FailureCode
		if failure == "" {
			failure = "agenthub_review_denied"
		}
		if _, err := a.deploymentStore().RecordExecutionStepOutcomeWithLease(ctx, job.TenantID,
			deployment.ID, deployment.Generation, job.ID, job.LeaseOwner, planDigest,
			executionStep.IdempotencyKey, "failed", "", failure); err != nil {
			return nil, true, false, err
		}
		return map[string]any{"step_id": executionStep.ID, "step_state": "failed",
			"failure_code": failure, "provider_dispatched": false}, true, false, nil
	}
	spec.Environment, err = a.deploymentRecoveryEnvironment(ctx, job, execution)
	if err != nil {
		if _, recordErr := a.deploymentStore().RecordExecutionStepOutcomeWithLease(ctx, job.TenantID,
			deployment.ID, deployment.Generation, job.ID, job.LeaseOwner, planDigest,
			executionStep.IdempotencyKey, "failed", "", "secret_resolution_failed"); recordErr != nil {
			return nil, true, false, recordErr
		}
		return map[string]any{"step_id": executionStep.ID, "step_state": "failed",
			"failure_code": "secret_resolution_failed", "provider_dispatched": false}, true, false, nil
	}
	step("claim_provider_workload_dispatch")
	if err := a.deploymentStore().ClaimExecutionStepDispatchWithLease(ctx, job.TenantID,
		deployment.ID, deployment.Generation, job.ID, job.LeaseOwner, planDigest, executionStep); err != nil {
		return nil, true, false, err
	}
	step("create_stopped_incus_oci_instance")
	guest, err := provider.CreateOCIApplicationStopped(ctx, spec)
	if err != nil {
		return nil, true, true, err
	}
	state, failure := "succeeded", ""
	if !strings.EqualFold(guest.Status, "Stopped") {
		state, failure = "unknown", "provider_state_mismatch"
	}
	if _, err := a.deploymentStore().RecordExecutionStepOutcomeWithLease(ctx, job.TenantID,
		deployment.ID, deployment.Generation, job.ID, job.LeaseOwner, planDigest,
		executionStep.IdempotencyKey, state, "", failure); err != nil {
		return nil, true, true, err
	}
	result := map[string]any{"step_id": executionStep.ID, "step_state": state,
		"provider_dispatched": true}
	if failure != "" {
		result["failure_code"] = failure
	}
	return result, true, true, nil
}
