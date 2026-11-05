package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strconv"

	"shippin.cloud/console/internal/jobs"
)

type deploymentWakeup struct {
	DeploymentID string `json:"deployment_id"`
	Generation   int64  `json:"generation"`
}

// relayDeploymentOutbox publishes identifier-only wake-ups to the existing
// durable jobs store. reconcileOnce holds the singleton leader lock while this
// runs. Enqueue is idempotent by live deployment generation, so replay after a
// publish/receipt crash returns the same job.
func (a *API) relayDeploymentOutbox(ctx context.Context) {
	items, err := a.deploymentStore().PendingOutbox(ctx, 100)
	if err != nil {
		log.Printf("reconcile: deployment outbox read: %v", err)
		return
	}
	for _, item := range items {
		payload, err := json.Marshal(deploymentWakeup{DeploymentID: item.DeploymentID, Generation: item.Generation})
		if err != nil {
			log.Printf("reconcile: deployment outbox payload %s: %v", item.ID, err)
			continue
		}
		targetID := item.DeploymentID + ":" + strconv.FormatInt(item.Generation, 10)
		job, _, err := a.jobs.Enqueue(ctx, jobs.Job{TenantID: item.TenantID, Kind: jobs.KindDeploymentReconcile,
			TargetType: "deployment_generation", TargetID: targetID, MaxAttempts: a.cfg.Jobs.MaxAttempts,
			Payload: payload, CreatedBy: item.CreatedBy})
		if err != nil {
			log.Printf("reconcile: deployment enqueue %s: %v", item.ID, err)
			continue
		}
		if err := a.deploymentStore().MarkOutboxPublished(ctx, item.ID, job.ID); err != nil {
			log.Printf("reconcile: deployment outbox receipt %s: %v", item.ID, err)
			continue
		}
		if a.runner != nil {
			a.runner.Wake()
		}
	}
}

// runDeploymentReconcileJob reloads the latest canonical desired generation.
// The current executor implements reviewed Incus storage and stopped-workload
// steps only. It persists an explicit unsupported result until the complete
// workload, power, health and lifecycle contract is implemented.
func (a *API) runDeploymentReconcileJob(ctx context.Context, job *jobs.Job, step func(string)) (any, error) {
	if job == nil || job.Kind != jobs.KindDeploymentReconcile || job.TargetType != "deployment_generation" || job.TargetID == "" {
		return nil, jobs.Permanent(errors.New("invalid deployment reconcile job"))
	}
	var wakeup deploymentWakeup
	if err := json.Unmarshal(job.Payload, &wakeup); err != nil || wakeup.DeploymentID == "" || wakeup.Generation < 1 {
		return nil, jobs.Permanent(errors.New("invalid deployment reconcile payload"))
	}
	if job.TargetID != wakeup.DeploymentID+":"+strconv.FormatInt(wakeup.Generation, 10) {
		return nil, jobs.Permanent(errors.New("deployment reconcile target does not match payload"))
	}
	step("load_current_desired_generation")
	deployment, err := a.deploymentStore().Get(ctx, job.TenantID, wakeup.DeploymentID)
	if err != nil {
		return nil, err
	}
	if deployment.Generation != wakeup.Generation {
		return map[string]any{"deployment_id": deployment.ID, "generation": wakeup.Generation,
			"outcome": "superseded", "provider_mutated": false}, nil
	}
	var recoveredStep map[string]any
	step("recover_ambiguous_provider_step")
	recoveredStep, recovered, err := a.recoverDeploymentIntent(ctx, job, deployment)
	if err != nil {
		return nil, err
	}
	var runtimeStep map[string]any
	providerMutated := false
	if !recovered {
		runtimeStep, _, providerMutated, err = a.executeIncusStorageStep(ctx, job, deployment, step)
		if err != nil {
			return nil, err
		}
	}
	mayProgressIncusRuntime := recoveredStep == nil || recoveredStep["step_state"] == "succeeded"
	if runtimeStep != nil && runtimeStep["step_state"] != "succeeded" {
		mayProgressIncusRuntime = false
	}
	if mayProgressIncusRuntime {
		workloadStep, handled, mutated, stepErr := a.executeIncusWorkloadStep(ctx, job, deployment, step)
		if stepErr != nil {
			return nil, stepErr
		}
		if handled {
			runtimeStep = workloadStep
			providerMutated = providerMutated || mutated
		}
	}
	if mayProgressIncusRuntime {
		powerStep, handled, mutated, stepErr := a.executeIncusPowerStep(ctx, job, deployment, step)
		if stepErr != nil {
			return nil, stepErr
		}
		if handled {
			runtimeStep = powerStep
			providerMutated = providerMutated || mutated
		}
	}
	step("application_runtime_unavailable")
	created, err := a.deploymentStore().RecordRuntimeUnavailableWithLease(ctx, job.TenantID, deployment.ID,
		job.CreatedBy, wakeup.Generation, job.ID, job.LeaseOwner)
	if err != nil {
		return nil, err
	}
	outcome := "runtime_unsupported"
	if !created {
		current, err := a.deploymentStore().Get(ctx, job.TenantID, deployment.ID)
		if err != nil {
			return nil, err
		}
		if current.Generation != wakeup.Generation {
			outcome = "superseded"
		}
	}
	result := map[string]any{"deployment_id": deployment.ID, "generation": wakeup.Generation,
		"outcome": outcome, "event_recorded": created, "provider_mutated": providerMutated}
	if recoveredStep != nil {
		result["recovery"] = recoveredStep
	}
	if runtimeStep != nil {
		result["runtime_step"] = runtimeStep
	}
	return result, nil
}
