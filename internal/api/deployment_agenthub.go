package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/agenthub"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/catalog"
	"shippin.cloud/console/internal/deployments"
	"shippin.cloud/console/internal/servers"
)

type deploymentReviewInput struct {
	Generation int64 `json:"generation"`
}

type deploymentReviewAction struct {
	Operation      string                 `json:"operation"`
	TenantID       string                 `json:"tenant_id"`
	DeploymentID   string                 `json:"deployment_id"`
	Generation     int64                  `json:"generation"`
	DesiredState   string                 `json:"desired_state"`
	TargetSystemID string                 `json:"target_system_id"`
	TargetProvider string                 `json:"target_provider"`
	ServiceID      string                 `json:"service_id"`
	DefinitionSHA  string                 `json:"definition_sha256"`
	Source         deployments.SourceView `json:"source"`
	Config         json.RawMessage        `json:"config"`
}

func (a *API) prepareDeploymentReview(w http.ResponseWriter, r *http.Request) {
	c, tenant, ok := a.deploymentTenantCtx(w, r)
	if !ok {
		return
	}
	if !c.canDecide(tenant) {
		auth.Problem(w, http.StatusForbidden, "A tenant owner or admin is required to request review.")
		return
	}
	if a.db == nil {
		deploymentUnavailable(w)
		return
	}
	key, ok := deploymentIdempotencyKey(w, r)
	if !ok {
		return
	}
	var input deploymentReviewInput
	if !decodeDeploymentBody(w, r, &input) {
		return
	}
	if input.Generation < 1 {
		auth.Problem(w, http.StatusBadRequest, "A positive deployment generation is required.")
		return
	}
	deployment, err := a.deploymentStore().Get(r.Context(), tenant.ID, r.PathValue("deploymentId"))
	if err != nil {
		writeDeploymentError(w, err)
		return
	}
	if deployment.Generation != input.Generation {
		writeDeploymentError(w, deployments.ErrGenerationConflict)
		return
	}
	action, actionSHA256, err := deploymentReviewSnapshot(tenant.ID, deployment)
	if err != nil {
		deploymentUnavailable(w)
		return
	}
	command := deploymentReviewCommand(deployment.ID, deployment.Generation, actionSHA256, deployment.DesiredState)
	path := "/shippin/deployments/" + deployment.ID + "/generations/" + strconv.FormatInt(deployment.Generation, 10) + "/review.json"
	reviewID := uuid.NewString()
	ticketID := "shp-" + strings.ReplaceAll(reviewID, "-", "")
	evidenceText, err := deploymentReviewEvidence(action, ticketID)
	if err != nil {
		writeHVProblem(w, http.StatusUnprocessableEntity, "review_evidence_too_large", "The deployment snapshot is too large to submit as review evidence.", "", "")
		return
	}

	review, _, err := a.deploymentStore().ReserveReview(r.Context(), tenant.ID, deployment.ID, deployment.Generation,
		c.id.Sub, key, reviewID, ticketID, actionSHA256)
	if err != nil {
		switch {
		case errors.Is(err, deployments.ErrGenerationConflict):
			writeDeploymentError(w, err)
		case errors.Is(err, deployments.ErrReviewConflict):
			writeHVProblem(w, http.StatusConflict, "review_action_conflict", "A different review snapshot already exists for this generation.", "", "")
		case errors.Is(err, deployments.ErrIdempotencyConflict):
			writeHVProblem(w, http.StatusUnprocessableEntity, "review_idempotency_conflict", "This review idempotency key was already used.", "", "")
		default:
			deploymentUnavailable(w)
		}
		return
	}
	if review.State == "pending_review" {
		writeStatus(w, http.StatusOK, map[string]any{"review": review, "agenthub_status": "pending_review"})
		return
	}
	if review.State == "failed" {
		review, err = a.deploymentStore().SetReviewState(r.Context(), review.ID, c.id.Sub, "preparing", "", "")
		if err != nil {
			deploymentUnavailable(w)
			return
		}
	}
	client, identity, err := a.deploymentReviewClient(r.Context(), deployment.TargetSystemID)
	if err != nil {
		a.failDeploymentReview(w, r, review, c.id.Sub, "agenthub_unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	if _, err := client.CheckSystemMapping(ctx, identity.AgentHubTenantID, identity.AgentHubMachineID, deployment.TargetSystemID); err != nil {
		a.failDeploymentReview(w, r, review, c.id.Sub, "agenthub_mapping_unverified")
		return
	}
	if _, err := client.CreateTicket(ctx, "shippin-"+review.ID+"-create", agenthub.TicketDraft{
		TenantID: identity.AgentHubTenantID, ID: review.AgentHubTicket,
		Title:   "Review deployment " + deployment.ID + " generation " + strconv.FormatInt(deployment.Generation, 10),
		Machine: identity.AgentHubMachineID,
		Command: command,
		Fields:  deploymentReviewFields(action, review.AgentHubTicket, path, actionSHA256),
		Lists:   deploymentReviewLists(action, path),
	}); err != nil {
		a.failDeploymentReview(w, r, review, c.id.Sub, "agenthub_ticket_create_failed")
		return
	}
	withEvidence, err := client.AddTicketEvidence(ctx, "shippin-"+review.ID+"-evidence", review.AgentHubTicket,
		agenthub.TicketEvidence{Kind: "snapshot", Path: path, Text: evidenceText})
	if err != nil {
		a.failDeploymentReview(w, r, review, c.id.Sub, "agenthub_evidence_failed")
		return
	}
	if withEvidence.TenantID != identity.AgentHubTenantID || withEvidence.Machine == nil ||
		*withEvidence.Machine != identity.AgentHubMachineID || withEvidence.Command != command ||
		withEvidence.Fields["shippin_action_sha256"] != actionSHA256 {
		a.failDeploymentReview(w, r, review, c.id.Sub, "agenthub_ticket_binding_failed")
		return
	}
	submitted, err := client.SubmitTicket(ctx, "shippin-"+review.ID+"-submit", review.AgentHubTicket)
	if err != nil {
		a.failDeploymentReview(w, r, review, c.id.Sub, "agenthub_submit_failed")
		return
	}
	if submitted.TenantID != identity.AgentHubTenantID || submitted.Machine == nil || *submitted.Machine != identity.AgentHubMachineID ||
		submitted.Command != command ||
		submitted.Fields["shippin_action_sha256"] != actionSHA256 ||
		submitted.Fields["shippin_system_id"] != deployment.TargetSystemID ||
		submitted.Fields["shippin_deployment_id"] != deployment.ID ||
		submitted.Fields["shippin_generation"] != strconv.FormatInt(deployment.Generation, 10) {
		a.failDeploymentReview(w, r, review, c.id.Sub, "agenthub_ticket_binding_failed")
		return
	}
	review, err = a.deploymentStore().SetReviewState(r.Context(), review.ID, c.id.Sub, "pending_review", submitted.SHA256, "")
	if err != nil {
		deploymentUnavailable(w)
		return
	}
	writeStatus(w, http.StatusAccepted, map[string]any{"review": review, "agenthub_status": submitted.Status})
}

func (a *API) authorizeDeploymentReview(w http.ResponseWriter, r *http.Request) {
	c, tenant, ok := a.deploymentTenantCtx(w, r)
	if !ok {
		return
	}
	if !c.canDecide(tenant) {
		auth.Problem(w, http.StatusForbidden, "A tenant owner or admin is required to verify deployment authorization.")
		return
	}
	if a.db == nil {
		deploymentUnavailable(w)
		return
	}
	key, ok := deploymentIdempotencyKey(w, r)
	if !ok {
		return
	}
	var input deploymentReviewInput
	if !decodeDeploymentBody(w, r, &input) {
		return
	}
	if input.Generation < 1 {
		auth.Problem(w, http.StatusBadRequest, "A positive deployment generation is required.")
		return
	}
	deployment, err := a.deploymentStore().Get(r.Context(), tenant.ID, r.PathValue("deploymentId"))
	if err != nil {
		writeDeploymentError(w, err)
		return
	}
	if deployment.Generation != input.Generation {
		writeDeploymentError(w, deployments.ErrGenerationConflict)
		return
	}
	result, err := a.authorizeDeploymentAction(r.Context(), tenant.ID, deployment, c.id.Sub, key)
	if err != nil {
		a.writeReviewDecisionError(w, err)
		return
	}
	if result.Status != http.StatusOK {
		if result.Event.ID == "" {
			writeHVProblem(w, result.Status, result.FailureCode, "The current generation does not have a submitted AgentHub review ticket.", "", "")
			return
		}
		writeStatus(w, result.Status, map[string]any{"event": result.Event, "message": "AgentHub did not authorize this deployment action."})
		return
	}
	writeStatus(w, http.StatusOK, map[string]any{
		"event": result.Event, "authorization": result.Event.ReviewProvenance,
		"message": "AgentHub authorization was recorded for audit. No provider operation was executed.",
	})
}

func (a *API) writeReviewDecisionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, deployments.ErrGenerationConflict):
		writeDeploymentError(w, err)
	case errors.Is(err, deployments.ErrReviewAuthorizationConflict), errors.Is(err, deployments.ErrIdempotencyConflict):
		writeHVProblem(w, http.StatusConflict, "agenthub_review_stale", "The deployment generation or AgentHub review changed. Refresh the review before retrying.", "", "")
	case errors.Is(err, deployments.ErrNotFound):
		writeDeploymentError(w, err)
	default:
		deploymentUnavailable(w)
	}
}

func (a *API) getDeploymentReview(w http.ResponseWriter, r *http.Request) {
	_, tenant, ok := a.deploymentTenantCtx(w, r)
	if !ok {
		return
	}
	deployment, err := a.deploymentStore().Get(r.Context(), tenant.ID, r.PathValue("deploymentId"))
	if err != nil {
		writeDeploymentError(w, err)
		return
	}
	review, err := a.deploymentStore().GetReview(r.Context(), tenant.ID, deployment.ID, deployment.Generation)
	if errors.Is(err, deployments.ErrNotFound) {
		writeJSON(w, map[string]any{"state": "not_requested", "generation": deployment.Generation})
		return
	}
	if err != nil {
		deploymentUnavailable(w)
		return
	}
	if review.State != "pending_review" {
		writeJSON(w, map[string]any{"review": review, "agenthub_status": "not_submitted"})
		return
	}
	connection, err := a.db.AgentHubConnection(r.Context())
	if err != nil || !connection.HasCredential || connection.BaseURL == "" {
		writeJSON(w, map[string]any{"review": review, "agenthub_status": "unavailable"})
		return
	}
	token, err := a.agentHubToken(r.Context())
	if err != nil {
		writeJSON(w, map[string]any{"review": review, "agenthub_status": "unavailable"})
		return
	}
	client, err := agenthub.NewClient(agenthub.Config{BaseURL: connection.BaseURL, Token: token, Timeout: 5 * time.Second})
	if err != nil {
		writeJSON(w, map[string]any{"review": review, "agenthub_status": "unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	ticket, err := client.GetTicket(ctx, review.AgentHubTicket)
	if err != nil {
		writeJSON(w, map[string]any{"review": review, "agenthub_status": "unavailable"})
		return
	}
	identity, err := a.serverStore().Identity(r.Context(), deployment.TargetSystemID)
	if err != nil || ticket.TenantID != identity.AgentHubTenantID || ticket.Machine == nil ||
		*ticket.Machine != identity.AgentHubMachineID || ticket.Fields["shippin_action_sha256"] != review.ActionSHA256 ||
		ticket.Fields["shippin_system_id"] != deployment.TargetSystemID || ticket.Fields["shippin_deployment_id"] != deployment.ID ||
		ticket.Fields["shippin_generation"] != strconv.FormatInt(deployment.Generation, 10) || ticket.SHA256 != review.TicketSHA256 {
		writeJSON(w, map[string]any{"review": review, "agenthub_status": "binding_mismatch"})
		return
	}
	writeJSON(w, map[string]any{"review": review, "agenthub_status": ticket.Status, "agenthub_ticket_sha256": ticket.SHA256})
}

func (a *API) deploymentReviewClient(ctx context.Context, systemID string) (*agenthub.Client, servers.SystemIdentity, error) {
	identity, err := a.serverStore().Identity(ctx, systemID)
	if err != nil || identity.AgentHubTenantID == "" || identity.AgentHubMachineID == "" {
		return nil, servers.SystemIdentity{}, errors.New("AgentHub machine mapping is not configured")
	}
	connection, err := a.db.AgentHubConnection(ctx)
	if err != nil || !connection.HasCredential || connection.BaseURL == "" {
		return nil, servers.SystemIdentity{}, errors.New("AgentHub connection is not configured")
	}
	token, err := a.agentHubToken(ctx)
	if err != nil {
		return nil, servers.SystemIdentity{}, errors.New("AgentHub credential is unavailable")
	}
	client, err := agenthub.NewClient(agenthub.Config{BaseURL: connection.BaseURL, Token: token, Timeout: 5 * time.Second})
	return client, identity, err
}

func (a *API) failDeploymentReview(w http.ResponseWriter, r *http.Request, review deployments.ReviewRequest, actor, code string) {
	failed, err := a.deploymentStore().SetReviewState(r.Context(), review.ID, actor, "failed", "", code)
	if err != nil {
		deploymentUnavailable(w)
		return
	}
	writeStatus(w, http.StatusBadGateway, map[string]any{"review": failed, "message": "AgentHub did not complete the review request. Retry this generation to resume the same ticket."})
}

func deploymentReviewCommand(deploymentID string, generation int64, actionSHA256 string, desired deployments.DesiredState) string {
	operation := "shippin:deployment.apply"
	if desired == deployments.DesiredAbsent {
		operation = "shippin:deployment.remove"
	}
	command, _ := json.Marshal(struct {
		Operation    string `json:"operation"`
		DeploymentID string `json:"deployment_id"`
		Generation   int64  `json:"generation"`
		ActionSHA256 string `json:"action_sha256"`
	}{operation, deploymentID, generation, actionSHA256})
	return string(command)
}

func deploymentReviewSnapshot(tenantID string, deployment *deployments.Deployment) (deploymentReviewAction, string, error) {
	if deployment == nil {
		return deploymentReviewAction{}, "", errors.New("deployment is required")
	}
	action := deploymentReviewAction{
		Operation: "deployment." + string(deployment.DesiredState), TenantID: tenantID,
		DeploymentID: deployment.ID, Generation: deployment.Generation, DesiredState: string(deployment.DesiredState),
		TargetSystemID: deployment.TargetSystemID, TargetProvider: string(deployment.TargetProvider),
		ServiceID: deployment.ServiceID, DefinitionSHA: deployment.DefinitionSHA256, Source: deployment.Source, Config: deployment.Config,
	}
	canonical, err := json.Marshal(action)
	if err != nil {
		return deploymentReviewAction{}, "", err
	}
	digest := sha256.Sum256(canonical)
	return action, hex.EncodeToString(digest[:]), nil
}

func deploymentReviewFields(action deploymentReviewAction, ticketID, evidencePath, actionSHA256 string) map[string]string {
	topology := fmt.Sprintf("Shippin system %s uses the %s provider for service %s; provider placement has not been executed or verified.",
		action.TargetSystemID, action.TargetProvider, action.ServiceID)
	blastRadius := fmt.Sprintf("Tenant %s deployment %s generation %d on target system %s only; no other tenant resources are in scope.",
		action.TenantID, action.DeploymentID, action.Generation, action.TargetSystemID)
	return map[string]string{
		"shippin_action_sha256":      actionSHA256,
		"shippin_agenthub_ticket_id": ticketID,
		"shippin_system_id":          action.TargetSystemID,
		"shippin_deployment_id":      action.DeploymentID,
		"shippin_generation":         strconv.FormatInt(action.Generation, 10),
		"topology":                   topology,
		"blast_radius":               blastRadius,
		"snapshot":                   evidencePath,
	}
}

func deploymentReviewLists(action deploymentReviewAction, evidencePath string) map[string][]string {
	research := evidencePath
	if strings.HasPrefix(action.Source.Reference, "https://") {
		research = action.Source.Reference
	}
	return map[string][]string{
		"research": []string{research},
		"verification": []string{
			"After a separately authorized provider operation exists, verify task completion and read the resulting resource back from the selected system.",
			"Keep observed state unknown until provider read-back and a fresh health observation confirm the requested state.",
		},
	}
}

func deploymentReviewEvidence(action deploymentReviewAction, ticketID string) (string, error) {
	redactedConfig := catalog.RedactSecretReferences(action.Config)
	artifact := struct {
		TicketID       string          `json:"ticket_id"`
		Operation      string          `json:"operation"`
		TenantID       string          `json:"tenant_id"`
		DeploymentID   string          `json:"deployment_id"`
		Generation     int64           `json:"generation"`
		DesiredState   string          `json:"desired_state"`
		TargetSystemID string          `json:"target_system_id"`
		TargetProvider string          `json:"target_provider"`
		ServiceID      string          `json:"service_id"`
		DefinitionSHA  string          `json:"definition_sha256"`
		Source         any             `json:"source"`
		Config         json.RawMessage `json:"config"`
	}{ticketID, action.Operation, action.TenantID, action.DeploymentID, action.Generation, action.DesiredState, action.TargetSystemID,
		action.TargetProvider, action.ServiceID, action.DefinitionSHA, action.Source, redactedConfig}
	text, err := json.Marshal(artifact)
	if err != nil || len(text) > 20_000 {
		return "", errors.New("bounded review evidence could not be encoded")
	}
	return string(text), nil
}
