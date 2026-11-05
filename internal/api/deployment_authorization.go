package api

import (
	"context"
	"errors"
	"net/http"
	"time"

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
	if deployment == nil {
		return deploymentAuthorizationResult{}, deployments.ErrNotFound
	}
	review, err := a.deploymentStore().GetReview(ctx, tenantID, deployment.ID, deployment.Generation)
	if err != nil {
		return deploymentAuthorizationResult{}, err
	}
	if review.State != "pending_review" || review.TicketSHA256 == "" {
		return deploymentAuthorizationResult{FailureCode: "agenthub_review_not_ready", Status: http.StatusConflict}, nil
	}
	_, actionSHA, err := deploymentReviewSnapshot(tenantID, deployment)
	if err != nil {
		return deploymentAuthorizationResult{}, err
	}
	if actionSHA != review.ActionSHA256 {
		return a.recordDeploymentAuthorizationDenial(ctx, tenantID, deployment, actor, idempotencyKey,
			"agenthub_review_stale", http.StatusConflict)
	}
	client, identity, err := a.deploymentReviewClient(ctx, deployment.TargetSystemID)
	if err != nil {
		return a.recordDeploymentAuthorizationDenial(ctx, tenantID, deployment, actor, idempotencyKey,
			"agenthub_review_unavailable", http.StatusBadGateway)
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
		return a.recordDeploymentAuthorizationDenial(ctx, tenantID, deployment, actor, idempotencyKey, code, status)
	}
	provenance := deployments.ReviewDecisionProvenance{
		DecisionID: decision.DecisionID, TenantID: decision.TenantID, MachineID: decision.MachineID,
		TicketID: decision.TicketID, TicketSHA256: decision.TicketSHA256, PolicySHA256: decision.PolicySHA256,
		PolicyVersion: decision.PolicyVersion, MatchedRules: decision.MatchedRules, ActionSHA256: actionSHA,
	}
	event, err := a.deploymentStore().RecordReviewDecision(ctx, tenantID, deployment.ID, deployment.Generation,
		actor, idempotencyKey, true, provenance, "")
	if err != nil {
		return deploymentAuthorizationResult{}, err
	}
	return deploymentAuthorizationResult{Event: event, Status: http.StatusOK}, nil
}

func (a *API) recordDeploymentAuthorizationDenial(ctx context.Context, tenantID string, deployment *deployments.Deployment,
	actor, idempotencyKey, failureCode string, status int) (deploymentAuthorizationResult, error) {
	event, err := a.deploymentStore().RecordReviewDecision(ctx, tenantID, deployment.ID, deployment.Generation,
		actor, idempotencyKey, false, deployments.ReviewDecisionProvenance{}, failureCode)
	if err != nil {
		return deploymentAuthorizationResult{}, err
	}
	return deploymentAuthorizationResult{Event: event, FailureCode: failureCode, Status: status}, nil
}
