package api

import (
	"context"
	"encoding/json"
	"errors"

	"shippin.cloud/console/internal/deployments"
	"shippin.cloud/console/internal/hv/incus"
	"shippin.cloud/console/internal/jobs"
)

type incusOCIRecoveryProvider interface {
	InspectOCIApplicationVolume(context.Context, incus.OCIApplicationSpec) (bool, error)
}

// recoverDeploymentIntent only observes the first unresolved step when it is
// the persistent-volume ensure operation. It never retries a provider write.
func (a *API) recoverDeploymentIntent(ctx context.Context, job *jobs.Job, deployment *deployments.Deployment) (map[string]any, bool, error) {
	plan, planDigest, unresolved, err := a.deploymentStore().UnresolvedExecutionStepsWithLease(ctx,
		job.TenantID, deployment.ID, deployment.Generation, job.ID, job.LeaseOwner)
	if err != nil {
		return nil, false, err
	}
	if len(unresolved) == 0 {
		return nil, false, nil
	}
	var step deployments.ExecutionStep
	for _, candidate := range plan.Steps {
		if candidate.IdempotencyKey == unresolved[0].IdempotencyKey {
			step = candidate
			break
		}
	}
	if step.ID == "" || step.Operation != "ensure_persistent_volume" || deployment.TargetProvider != deployments.ProviderIncus {
		return nil, false, nil
	}
	if _, _, err := a.deploymentStore().ClaimExecutionStepRecoveryWithLease(ctx, job.TenantID, deployment.ID,
		deployment.Generation, job.ID, job.LeaseOwner, planDigest, step); err != nil {
		return nil, false, err
	}
	snapshot, err := a.deploymentStore().ExecutionRecoverySnapshotWithLease(ctx, job.TenantID, deployment.ID,
		deployment.Generation, job.ID, job.LeaseOwner, planDigest, step.IdempotencyKey)
	if err != nil {
		return nil, false, err
	}
	spec, err := incusOCIRecoverySpec(snapshot)
	if err != nil {
		return nil, false, err
	}
	targetConfig, err := json.Marshal(deployment.TargetConfig)
	if err != nil {
		return nil, false, deployments.ErrInvalidConfig
	}
	if err := a.validateDeploymentTargetConfig(ctx, job.TenantID, deployment.TargetSystemID, targetConfig); err != nil {
		return nil, false, err
	}
	endpoint, err := a.visibleEndpoint(ctx, job.TenantID, deployment.TargetSystemID)
	if err != nil || endpoint.Kind != string(deployments.ProviderIncus) {
		return nil, false, deployments.ErrTargetUnavailable
	}
	var provider incusOCIRecoveryProvider
	if a.openIncusOCIRecovery != nil {
		provider, err = a.openIncusOCIRecovery(endpoint)
	} else {
		provider, err = a.openIncus(endpoint)
	}
	if err != nil {
		return nil, false, err
	}
	exists, err := provider.InspectOCIApplicationVolume(ctx, spec)
	if err != nil {
		return nil, false, err
	}
	state, failureCode := "succeeded", ""
	if !exists {
		// Absence does not prove that the original mutation did not complete
		// transiently or that retrying it is authorized. Close as unknown.
		state, failureCode = "unknown", "provider_state_absent"
	}
	if _, err := a.deploymentStore().RecordExecutionStepOutcomeWithLease(ctx, job.TenantID,
		deployment.ID, deployment.Generation, job.ID, job.LeaseOwner, planDigest,
		step.IdempotencyKey, state, "", failureCode); err != nil {
		return nil, false, err
	}
	return map[string]any{"step_id": step.ID, "step_state": state, "provider_mutated": false}, true, nil
}

func incusOCIRecoverySpec(snapshot deployments.ExecutionRecoverySnapshot) (incus.OCIApplicationSpec, error) {
	runtime := snapshot.Definition.Runtime
	if runtime.Kind != "incus_oci_container" || runtime.Resources == nil || runtime.Storage == nil || runtime.Network == nil {
		return incus.OCIApplicationSpec{}, errors.New("deployment recovery runtime recipe is incomplete")
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

var _ incusOCIRecoveryProvider = (*incus.Client)(nil)
