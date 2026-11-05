package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/agenthub"
	"shippin.cloud/console/internal/deployments"
)

type deploymentAuthorizationResult struct {
	Event       deployments.DeploymentEvent
	FailureCode string
	Status      int
}

// authorizeDeploymentAction is the single authorization path for a deployment
// generation. It reloads its submitted review, recomputes the exact action,
// verifies the configured system mapping, obtains AgentHub's current decision,
// and persists only safe provenance or a normalized denial. Callers must still
// fence the generation/worker lease before every provider side effect.
func (a *API) authorizeDeploymentAction(ctx context.Context, tenantID string, deployment *deployments.Deployment,
	actor, idempotencyKey string) (deploymentAuthorizationResult, error) {
	return a.authorizeDeploymentActionWithFence(ctx, tenantID, deployment, actor, idempotencyKey, nil)
}

// authorizeDeploymentActionWithLease is the worker variant of the same exact
// AgentHub decision path. The job ID is the idempotency key, so retries of one
// leased job cannot append duplicate review events. Call it only immediately
// before dispatching an implemented provider operation.
func (a *API) authorizeDeploymentActionWithLease(ctx context.Context, tenantID string,
	deployment *deployments.Deployment, actor, jobID, leaseOwner string) (deploymentAuthorizationResult, error) {
	if _, err := uuid.Parse(jobID); err != nil || strings.TrimSpace(leaseOwner) == "" || len(leaseOwner) > 255 {
		return deploymentAuthorizationResult{}, deployments.ErrInvalidRequest
	}
	if deployment == nil {
		return deploymentAuthorizationResult{}, deployments.ErrNotFound
	}
	if err := a.deploymentStore().CheckReviewDecisionLease(ctx, tenantID, deployment.ID,
		deployment.Generation, jobID, leaseOwner); err != nil {
		return deploymentAuthorizationResult{}, err
	}
	return a.authorizeDeploymentActionWithFence(ctx, tenantID, deployment, actor, jobID,
		&deploymentAuthorizationFence{jobID: jobID, leaseOwner: leaseOwner})
}

type deploymentAuthorizationFence struct {
	jobID      string
	leaseOwner string
}

func (a *API) authorizeDeploymentActionWithFence(ctx context.Context, tenantID string, deployment *deployments.Deployment,
	actor, idempotencyKey string, fence *deploymentAuthorizationFence) (deploymentAuthorizationResult, error) {
	if deployment == nil {
		return deploymentAuthorizationResult{}, deployments.ErrNotFound
	}
	review, err := a.deploymentStore().GetReview(ctx, tenantID, deployment.ID, deployment.Generation)
	if err != nil {
		return deploymentAuthorizationResult{}, err
	}
	if review.State != "pending_review" || review.TicketSHA256 == "" || review.ExecutionPlanVersion != deployments.ExecutionPlanVersion || review.ExecutionPlanSHA256 == "" {
		return deploymentAuthorizationResult{FailureCode: "agenthub_review_not_ready", Status: http.StatusConflict}, nil
	}
	currentAction, planSHA, actionSHA, err := deploymentReviewSnapshot(tenantID, deployment)
	if err != nil {
		return deploymentAuthorizationResult{}, err
	}
	if currentAction.ExecutionPlan.Version != review.ExecutionPlanVersion || planSHA != review.ExecutionPlanSHA256 || actionSHA != review.ActionSHA256 {
		return a.recordDeploymentAuthorizationDenial(ctx, tenantID, deployment, actor, idempotencyKey,
			"agenthub_review_stale", http.StatusConflict, fence)
	}
	client, identity, err := a.deploymentReviewClient(ctx, deployment.TargetSystemID)
	if err != nil {
		return a.recordDeploymentAuthorizationDenial(ctx, tenantID, deployment, actor, idempotencyKey,
			"agenthub_review_unavailable", http.StatusBadGateway, fence)
	}
	decisionCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	tool := "shippin:deployment.apply"
	if deployment.DesiredState == deployments.DesiredAbsent {
		tool = "shippin:deployment.remove"
	}
	decision, err := client.Authorize(decisionCtx, agenthub.Request{
		TenantID: identity.AgentHubTenantID, MachineID: identity.AgentHubMachineID,
		TicketID: review.AgentHubTicket, Tool: tool,
		Command:      deploymentReviewCommand(deployment.ID, deployment.Generation, actionSHA, deployment.DesiredState),
		ActionSHA256: actionSHA,
	})
	if err != nil {
		code, status := "agenthub_review_unavailable", http.StatusBadGateway
		if errors.Is(err, agenthub.ErrDenied) {
			code, status = "agenthub_review_denied", http.StatusConflict
		}
		return a.recordDeploymentAuthorizationDenial(ctx, tenantID, deployment, actor, idempotencyKey, code, status, fence)
	}
	provenance := deployments.ReviewDecisionProvenance{
		DecisionID: decision.DecisionID, TenantID: decision.TenantID, MachineID: decision.MachineID,
		TicketID: decision.TicketID, TicketSHA256: decision.TicketSHA256, PolicySHA256: decision.PolicySHA256,
		PolicyVersion: decision.PolicyVersion, MatchedRules: decision.MatchedRules, ActionSHA256: actionSHA,
	}
	event, err := a.recordDeploymentAuthorization(ctx, tenantID, deployment, actor, idempotencyKey, true, provenance, "", fence)
	if err != nil {
		return deploymentAuthorizationResult{}, err
	}
	return deploymentAuthorizationResult{Event: event, Status: http.StatusOK}, nil
}

func (a *API) recordDeploymentAuthorizationDenial(ctx context.Context, tenantID string, deployment *deployments.Deployment,
	actor, idempotencyKey, failureCode string, status int, fence *deploymentAuthorizationFence) (deploymentAuthorizationResult, error) {
	event, err := a.recordDeploymentAuthorization(ctx, tenantID, deployment, actor, idempotencyKey, false,
		deployments.ReviewDecisionProvenance{}, failureCode, fence)
	if err != nil {
		return deploymentAuthorizationResult{}, err
	}
	return deploymentAuthorizationResult{Event: event, FailureCode: failureCode, Status: status}, nil
}

func (a *API) recordDeploymentAuthorization(ctx context.Context, tenantID string, deployment *deployments.Deployment,
	actor, idempotencyKey string, allowed bool, provenance deployments.ReviewDecisionProvenance,
	failureCode string, fence *deploymentAuthorizationFence) (deployments.DeploymentEvent, error) {
	if fence == nil {
		return a.deploymentStore().RecordReviewDecision(ctx, tenantID, deployment.ID, deployment.Generation,
			actor, idempotencyKey, allowed, provenance, failureCode)
	}
	return a.deploymentStore().RecordReviewDecisionWithLease(ctx, tenantID, deployment.ID, deployment.Generation,
		actor, idempotencyKey, fence.jobID, fence.leaseOwner, allowed, provenance, failureCode)
}
