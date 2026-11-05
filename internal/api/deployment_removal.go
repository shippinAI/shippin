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

type incusOCIRemovalProvider interface {
	InspectOCIApplicationPowerState(context.Context, incus.OCIApplicationSpec) (hv.Guest, bool, error)
	InspectOCIApplicationSnapshots(context.Context, incus.OCIApplicationSpec) (bool, error)
	StopOCIApplication(context.Context, incus.OCIApplicationSpec) error
	DeleteOCIApplicationStopped(context.Context, incus.OCIApplicationSpec) error
}

// executeIncusRemovalSteps executes a reviewed desired-absent plan in order.
// Every mutation has its own intent, current AgentHub decision, one-time
// dispatch claim and provider read-back. The deployment-owned volume is kept.
func (a *API) executeIncusRemovalSteps(ctx context.Context, job *jobs.Job, deployment *deployments.Deployment,
	step func(string)) (map[string]any, bool, bool, error) {
	if deployment.TargetProvider != deployments.ProviderIncus || deployment.DesiredState != deployments.DesiredAbsent {
		return nil, false, false, nil
	}
	plan, planDigest, unresolved, err := a.deploymentStore().UnresolvedExecutionStepsWithLease(ctx,
		job.TenantID, deployment.ID, deployment.Generation, job.ID, job.LeaseOwner)
	if err != nil {
		return nil, false, false, err
	}
	if len(unresolved) != 0 {
		// Only the read-only recovery path may close a previously dispatched intent.
		return nil, false, false, nil
	}
	review, err := a.deploymentStore().GetReview(ctx, job.TenantID, deployment.ID, deployment.Generation)
	if err != nil {
		if errors.Is(err, deployments.ErrNotFound) {
			return nil, false, false, nil
		}
		return nil, false, false, err
	}
	if review.State != "pending_review" || review.ExecutionPlanVersion != plan.Version || review.ExecutionPlanSHA256 != planDigest {
		return nil, false, false, nil
	}
	execution, err := a.deploymentStore().ExecutionSnapshotWithLease(ctx, job.TenantID, deployment.ID,
		deployment.Generation, job.ID, job.LeaseOwner, planDigest)
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
	var provider incusOCIRemovalProvider
	if a.openIncusOCIExecutor != nil {
		provider, err = a.openIncusOCIExecutor(endpoint)
	} else {
		provider, err = a.openIncus(endpoint)
	}
	if err != nil {
		return nil, false, false, err
	}
	if provider == nil {
		return nil, false, false, errors.New("Incus OCI removal executor is unavailable")
	}

	var result map[string]any
	mutated := false
	for _, executionStep := range plan.Steps {
		intent, started, err := a.deploymentStore().BeginExecutionStepWithLease(ctx, job.TenantID,
			deployment.ID, deployment.Generation, job.ID, job.LeaseOwner, planDigest, executionStep)
		if err != nil {
			return nil, true, mutated, err
		}
		if !started {
			switch intent.State {
			case "succeeded":
				result = map[string]any{"step_id": intent.StepID, "step_state": intent.State, "provider_dispatched": false}
				continue
			case "intent":
				return map[string]any{"step_id": intent.StepID, "step_state": intent.State,
					"provider_dispatched": false}, true, mutated, nil
			default:
				return map[string]any{"step_id": intent.StepID, "step_state": intent.State,
					"failure_code": intent.FailureCode, "provider_dispatched": false}, true, mutated, nil
			}
		}

		if executionStep.Operation == "verify_instance_absent" {
			step("verify_incus_oci_instance_absent")
			_, exists, inspectErr := provider.InspectOCIApplicationPowerState(ctx, spec)
			if inspectErr != nil {
				return nil, true, mutated, inspectErr
			}
			state, failure := "succeeded", ""
			if exists {
				state, failure = "unknown", "provider_state_present"
			}
			result, err = a.recordIncusRemovalOutcome(ctx, job, deployment, planDigest, executionStep, state, failure, false)
			if err != nil {
				return nil, true, mutated, err
			}
			if state != "succeeded" {
				return result, true, mutated, nil
			}
			// This durable observation is idempotent across worker replay and is the
			// only path that moves the deployment's observed state to absent.
			eventKey := uuid.NewSHA1(uuid.NameSpaceURL, []byte("shippin-deployment-absent:"+executionStep.IdempotencyKey)).String()
			_, err = a.deploymentStore().RecordOperationEventWithLease(ctx, job.TenantID, deployment.ID,
				job.ID, job.LeaseOwner, deployments.DeploymentEvent{TenantID: job.TenantID,
					DeploymentID: deployment.ID, Generation: deployment.Generation,
					Type: deployments.EventProviderObserved, ObservedState: deployments.ObservedAbsent,
					IdempotencyKey: eventKey, Actor: job.CreatedBy})
			if err != nil {
				return nil, true, mutated, err
			}
			continue
		}

		step("authorize_provider_removal_step")
		authorization, err := a.authorizeDeploymentActionWithLease(ctx, job.TenantID,
			execution.Deployment, job.CreatedBy, job.ID, job.LeaseOwner)
		if err != nil {
			return nil, true, mutated, err
		}
		if authorization.Status != http.StatusOK {
			failure := authorization.FailureCode
			if failure == "" {
				failure = "agenthub_review_denied"
			}
			result, err = a.recordIncusRemovalOutcome(ctx, job, deployment, planDigest, executionStep, "failed", failure, false)
			return result, true, mutated, err
		}

		guest, exists, err := provider.InspectOCIApplicationPowerState(ctx, spec)
		if err != nil {
			return nil, true, mutated, err
		}
		if !exists {
			result, err = a.recordIncusRemovalOutcome(ctx, job, deployment, planDigest, executionStep, "succeeded", "", false)
			if err != nil {
				return nil, true, mutated, err
			}
			continue
		}

		switch executionStep.Operation {
		case "stop_owned_instance":
			if strings.EqualFold(guest.Status, "Stopped") {
				result, err = a.recordIncusRemovalOutcome(ctx, job, deployment, planDigest, executionStep, "succeeded", "", false)
				if err != nil {
					return nil, true, mutated, err
				}
				continue
			}
			if !strings.EqualFold(guest.Status, "Running") {
				result, err = a.recordIncusRemovalOutcome(ctx, job, deployment, planDigest, executionStep, "unknown", "provider_state_mismatch", false)
				return result, true, mutated, err
			}
			if err := a.deploymentStore().ClaimExecutionStepDispatchWithLease(ctx, job.TenantID,
				deployment.ID, deployment.Generation, job.ID, job.LeaseOwner, planDigest, executionStep); err != nil {
				return nil, true, mutated, err
			}
			step("stop_owned_incus_oci_instance")
			if err := provider.StopOCIApplication(ctx, spec); err != nil {
				return nil, true, true, err
			}
			mutated = true
			guest, exists, err = provider.InspectOCIApplicationPowerState(ctx, spec)
			if err != nil {
				return nil, true, mutated, err
			}
			state, failure := "succeeded", ""
			if !exists || !strings.EqualFold(guest.Status, "Stopped") {
				state, failure = "unknown", "provider_state_mismatch"
			}
			result, err = a.recordIncusRemovalOutcome(ctx, job, deployment, planDigest, executionStep, state, failure, true)
		case "delete_owned_instance_preserve_volume":
			if !strings.EqualFold(guest.Status, "Stopped") {
				result, err = a.recordIncusRemovalOutcome(ctx, job, deployment, planDigest, executionStep, "unknown", "provider_state_mismatch", false)
				return result, true, mutated, err
			}
			hasSnapshots, err := provider.InspectOCIApplicationSnapshots(ctx, spec)
			if err != nil {
				return nil, true, mutated, err
			}
			if hasSnapshots {
				result, err = a.recordIncusRemovalOutcome(ctx, job, deployment, planDigest, executionStep, "failed", "snapshots_present", false)
				return result, true, mutated, err
			}
			if err := a.deploymentStore().ClaimExecutionStepDispatchWithLease(ctx, job.TenantID,
				deployment.ID, deployment.Generation, job.ID, job.LeaseOwner, planDigest, executionStep); err != nil {
				return nil, true, mutated, err
			}
			step("delete_owned_incus_oci_instance_preserve_volume")
			if err := provider.DeleteOCIApplicationStopped(ctx, spec); err != nil {
				return nil, true, true, err
			}
			mutated = true
			_, exists, err = provider.InspectOCIApplicationPowerState(ctx, spec)
			if err != nil {
				return nil, true, mutated, err
			}
			state, failure := "succeeded", ""
			if exists {
				state, failure = "unknown", "provider_state_present"
			}
			result, err = a.recordIncusRemovalOutcome(ctx, job, deployment, planDigest, executionStep, state, failure, true)
		default:
			return nil, true, mutated, deployments.ErrInvalidRequest
		}
		if err != nil {
			return nil, true, mutated, err
		}
		if result["step_state"] != "succeeded" {
			return result, true, mutated, nil
		}
	}
	return result, true, mutated, nil
}

func (a *API) recordIncusRemovalOutcome(ctx context.Context, job *jobs.Job, deployment *deployments.Deployment,
	planDigest string, executionStep deployments.ExecutionStep, state, failure string, dispatched bool) (map[string]any, error) {
	if _, err := a.deploymentStore().RecordExecutionStepOutcomeWithLease(ctx, job.TenantID,
		deployment.ID, deployment.Generation, job.ID, job.LeaseOwner, planDigest,
		executionStep.IdempotencyKey, state, "", failure); err != nil {
		return nil, err
	}
	result := map[string]any{"step_id": executionStep.ID, "step_state": state, "provider_dispatched": dispatched}
	if failure != "" {
		result["failure_code"] = failure
	}
	return result, nil
}

var _ incusOCIRemovalProvider = (*incus.Client)(nil)
