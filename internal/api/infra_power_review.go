package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/agenthub"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/servers"
)

const infraPowerReviewHeader = "X-AgentHub-Review-ID"

type infraPowerAction struct {
	Operation   string `json:"operation"`
	ServerID    string `json:"server_id"`
	Provider    string `json:"provider"`
	GuestID     string `json:"guest_id"`
	Verb        string `json:"verb"`
	EndpointSHA string `json:"endpoint_config_sha256"`
	ChangeNote  string `json:"change_note"`
	Actor       string `json:"actor"`
	ConfirmSelf bool   `json:"confirm_self"`
}

func (a *API) prepareInfraPowerReview(w http.ResponseWriter, r *http.Request, c *caller) {
	a.prepareInfraPowerReviewFor(w, r, c, "")
}

func (a *API) prepareInfraPowerReviewFor(w http.ResponseWriter, r *http.Request, c *caller, tenant string) {
	if a.db == nil || c == nil || c.id == nil {
		infraReviewUnavailable(w)
		return
	}
	serverID, guest, verb := r.PathValue("id"), r.PathValue("guest"), r.PathValue("verb")
	if serverID == "" {
		serverID = r.PathValue("serverId")
	}
	if err := validPowerVerb(verb); err != nil {
		auth.Problem(w, http.StatusBadRequest, "Verb must be start, stop, shutdown, or reboot.")
		return
	}
	if len(guest) == 0 || len(guest) > 255 || strings.TrimSpace(guest) != guest || strings.IndexFunc(guest, unicode.IsControl) >= 0 {
		auth.Problem(w, http.StatusBadRequest, "Guest ID must be between 1 and 255 characters and contain no control characters.")
		return
	}
	key, err := uuid.Parse(strings.TrimSpace(r.Header.Get("Idempotency-Key")))
	if err != nil {
		auth.Problem(w, http.StatusBadRequest, "Send a UUID Idempotency-Key for this review request.")
		return
	}
	ep, err := a.powerEndpoint(r.Context(), tenant, serverID)
	if err != nil {
		infraErr(w, err)
		return
	}
	driver, err := a.openDriver(ep)
	if err != nil {
		infraErr(w, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	current, err := driver.Guest(ctx, guest)
	if err != nil {
		infraErr(w, err)
		return
	}
	if !a.verifyInfraPowerTarget(w, r, c, current, guest, verb) {
		return
	}
	endpointSHA := infraEndpointSHA(ep)
	action := infraPowerAction{"guest_power." + verb, serverID, ep.Kind, guest, verb, endpointSHA, c.note, c.id.Sub, r.URL.Query().Get("confirm") == "self"}
	actionBytes, err := json.Marshal(action)
	if err != nil {
		infraReviewUnavailable(w)
		return
	}
	sha := sha256.Sum256(actionBytes)
	actionSHA := hex.EncodeToString(sha[:])
	id, err := uuid.NewV7()
	if err != nil {
		infraReviewUnavailable(w)
		return
	}
	ticketID := "shp-" + strings.ReplaceAll(id.String(), "-", "")
	review, created, err := a.serverStore().ReservePowerReview(r.Context(), servers.PowerReview{
		ID: id.String(), ServerID: serverID, Guest: guest, Verb: verb, Provider: ep.Kind,
		EndpointSHA256: endpointSHA, ActionSHA256: actionSHA, IdempotencyKey: key.String(),
		AgentHubTicket: ticketID, State: "preparing", Actor: c.id.Sub, ActorName: displayName(c), Note: c.note,
		ReviewWindowMinutes: a.powerReviewTTLMinutes(r.Context()),
	})
	if errors.Is(err, servers.ErrPowerReviewConflict) {
		writeHVProblem(w, http.StatusConflict, "review_action_conflict", "This idempotency key belongs to a different guest power action.", "", "")
		return
	}
	if err != nil {
		infraReviewUnavailable(w)
		return
	}
	if !created && review.State != "preparing" && review.State != "failed" {
		writeJSON(w, map[string]any{"review": review})
		return
	}
	if !created && review.State == "failed" {
		review, err = a.serverStore().SetPowerReviewState(r.Context(), review.ID, "preparing", "", "")
		if err != nil {
			infraReviewUnavailable(w)
			return
		}
	}
	client, identity, err := a.deploymentReviewClient(r.Context(), serverID)
	if err != nil {
		a.failInfraPowerReview(w, r, review, "agenthub_unavailable")
		return
	}
	command := infraPowerCommand(actionSHA, action)
	path := "/shippin/infrastructure/" + serverID + "/power-reviews/" + review.ID + ".json"
	evidence, _ := json.Marshal(action)
	ctx, cancel = context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	if _, err = client.CheckSystemMapping(ctx, identity.AgentHubTenantID, identity.AgentHubMachineID, serverID); err != nil {
		a.failInfraPowerReview(w, r, review, "agenthub_mapping_unverified")
		return
	}
	fields := map[string]string{"shippin_action_sha256": actionSHA, "shippin_agenthub_ticket_id": ticketID,
		"shippin_system_id": serverID, "shippin_guest_id": guest, "shippin_guest_verb": verb,
		"shippin_endpoint_config_sha256": endpointSHA, "snapshot": path}
	createdTicket, err := client.CreateTicket(ctx, "shippin-"+review.ID+"-create", agenthub.TicketDraft{
		TenantID: identity.AgentHubTenantID, ID: ticketID, Title: fmt.Sprintf("Review guest power action on system %s", serverID),
		Machine: identity.AgentHubMachineID, Command: command, Fields: fields,
	})
	if err != nil || !infraPowerTicketMatches(createdTicket, identity.AgentHubTenantID, identity.AgentHubMachineID, ticketID, command, actionSHA, serverID, guest, verb, endpointSHA) {
		a.failInfraPowerReview(w, r, review, "agenthub_ticket_create_failed")
		return
	}
	withEvidence, err := client.AddTicketEvidence(ctx, "shippin-"+review.ID+"-evidence", ticketID,
		agenthub.TicketEvidence{Kind: "snapshot", Path: path, Text: string(evidence)})
	if err != nil || !infraPowerTicketMatches(withEvidence, identity.AgentHubTenantID, identity.AgentHubMachineID, ticketID, command, actionSHA, serverID, guest, verb, endpointSHA) {
		a.failInfraPowerReview(w, r, review, "agenthub_evidence_failed")
		return
	}
	submitted, err := client.SubmitTicket(ctx, "shippin-"+review.ID+"-submit", ticketID)
	if err != nil || submitted.Status != "pending_review" || !infraPowerTicketMatches(submitted, identity.AgentHubTenantID, identity.AgentHubMachineID, ticketID, command, actionSHA, serverID, guest, verb, endpointSHA) {
		a.failInfraPowerReview(w, r, review, "agenthub_ticket_binding_failed")
		return
	}
	review, err = a.serverStore().SetPowerReviewState(r.Context(), review.ID, "pending_review", submitted.SHA256, "")
	if err != nil {
		infraReviewUnavailable(w)
		return
	}
	writeStatus(w, http.StatusAccepted, map[string]any{"review": review, "agenthub_status": submitted.Status})
}

func (a *API) getInfraPowerReview(w http.ResponseWriter, r *http.Request, c *caller) {
	a.getInfraPowerReviewFor(w, r, c, "")
}

func (a *API) getInfraPowerReviewFor(w http.ResponseWriter, r *http.Request, _ *caller, tenant string) {
	if a.db == nil {
		infraReviewUnavailable(w)
		return
	}
	serverID, reviewID := r.PathValue("id"), r.PathValue("reviewID")
	if serverID == "" {
		serverID = r.PathValue("serverId")
	}
	if tenant != "" {
		if _, err := a.visibleEndpoint(r.Context(), tenant, serverID); err != nil {
			auth.Problem(w, http.StatusNotFound, "This server is not available to this organization.")
			return
		}
	}
	if _, err := a.serverStore().ExpirePowerReviewIfDue(r.Context(), reviewID, serverID); err != nil {
		infraReviewUnavailable(w)
		return
	}
	review, err := a.serverStore().GetPowerReview(r.Context(), reviewID, serverID)
	if err != nil {
		infraErr(w, err)
		return
	}
	if review.Guest != r.PathValue("guest") {
		auth.Problem(w, http.StatusNotFound, "This review is not available for that guest.")
		return
	}
	if review.State != "pending_review" {
		writeJSON(w, map[string]any{"review": review, "agenthub_status": review.State})
		return
	}
	endpoint, err := a.powerEndpoint(r.Context(), tenant, serverID)
	if err != nil || infraEndpointSHA(endpoint) != review.EndpointSHA256 {
		writeJSON(w, map[string]any{"review": review, "agenthub_status": "stale"})
		return
	}
	client, identity, err := a.deploymentReviewClient(r.Context(), serverID)
	if err != nil {
		writeJSON(w, map[string]any{"review": review, "agenthub_status": "unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	ticket, err := client.GetTicket(ctx, review.AgentHubTicket)
	if err != nil || identity.AgentHubTenantID != ticket.TenantID || identity.AgentHubMachineID == "" || ticket.Machine == nil || *ticket.Machine != identity.AgentHubMachineID ||
		ticket.Fields["shippin_action_sha256"] != review.ActionSHA256 || ticket.Fields["shippin_system_id"] != serverID || ticket.Fields["shippin_guest_id"] != review.Guest ||
		ticket.Fields["shippin_guest_verb"] != review.Verb || ticket.Fields["shippin_endpoint_config_sha256"] != review.EndpointSHA256 || ticket.SHA256 != review.TicketSHA256 {
		writeJSON(w, map[string]any{"review": review, "agenthub_status": "binding_mismatch"})
		return
	}
	writeJSON(w, map[string]any{"review": review, "agenthub_status": ticket.Status, "agenthub_ticket_sha256": ticket.SHA256})
}

func (a *API) powerReviewTTLMinutes(ctx context.Context) int {
	minutes := 30
	if configured, ok := a.platformValue(ctx, "infra.power_review_ttl_minutes").(float64); ok && configured >= 1 && configured <= 1440 {
		minutes = int(configured)
	}
	return minutes
}

// externalReviewConnected says this installation has connected an outside reviewer. Until it has, a guest
// action rests on the permission, written reason and audit entry that came before it. A connection that
// cannot be read counts as connected, so a database fault never skips a configured review.
func (a *API) externalReviewConnected(ctx context.Context) bool {
	if a.db == nil {
		return false
	}
	connection, err := a.db.AgentHubConnection(ctx)
	if errors.Is(err, db.ErrNotFound) {
		return false
	}
	return err != nil || (connection.BaseURL != "" && connection.HasCredential)
}

// executeInfraPower runs a guest power action, through the outside reviewer when one is connected.
func (a *API) executeInfraPower(w http.ResponseWriter, r *http.Request, c *caller, tenant string, endpoint servers.Endpoint, serverID, guest, verb string) bool {
	if a.externalReviewConnected(r.Context()) {
		return a.executeInfraPowerReview(w, r, c, tenant, endpoint, serverID, guest, verb)
	}
	driver, err := a.openDriver(endpoint)
	if err != nil {
		infraErr(w, err)
		return false
	}
	current, err := driver.Guest(r.Context(), guest)
	if err != nil {
		infraErr(w, err)
		return false
	}
	if !a.verifyInfraPowerTarget(w, r, c, current, guest, verb) {
		return false
	}
	g, err := a.tracked(r, c, serverID, guest, verb, func(ctx context.Context) (hv.Guest, error) { return runVerb(ctx, driver, verb, guest) })
	if err != nil {
		infraErr(w, err)
		return false
	}
	writeJSON(w, g)
	return true
}

func (a *API) executeInfraPowerReview(w http.ResponseWriter, r *http.Request, c *caller, tenant string, endpoint servers.Endpoint, serverID, guest, verb string) bool {
	if a.db == nil || c == nil || c.id == nil {
		infraReviewUnavailable(w)
		return false
	}
	reviewID := strings.TrimSpace(r.Header.Get(infraPowerReviewHeader))
	review, err := a.serverStore().GetPowerReview(r.Context(), reviewID, serverID)
	if err != nil || review.State != "pending_review" || review.Guest != guest || review.Verb != verb || review.Actor != c.id.Sub {
		writeHVProblem(w, http.StatusConflict, "agenthub_review_required", "Request a current AgentHub review for this exact guest action first.", "", "")
		return false
	}
	if endpoint.ID != serverID || infraEndpointSHA(endpoint) != review.EndpointSHA256 {
		writeHVProblem(w, http.StatusConflict, "agenthub_review_stale", "The provider connection changed after review was requested.", "", "")
		return false
	}
	action := infraPowerAction{"guest_power." + verb, serverID, endpoint.Kind, guest, verb, review.EndpointSHA256, c.note, c.id.Sub, r.URL.Query().Get("confirm") == "self"}
	data, _ := json.Marshal(action)
	digest := sha256.Sum256(data)
	actionSHA := hex.EncodeToString(digest[:])
	if actionSHA != review.ActionSHA256 {
		writeHVProblem(w, http.StatusConflict, "agenthub_review_stale", "The requested action differs from the action sent to AgentHub.", "", "")
		return false
	}
	driver, err := a.openDriver(endpoint)
	if err != nil {
		infraErr(w, err)
		return false
	}
	current, err := driver.Guest(r.Context(), guest)
	if err != nil {
		infraErr(w, err)
		return false
	}
	if !a.verifyInfraPowerTarget(w, r, c, current, guest, verb) {
		return false
	}
	client, identity, err := a.deploymentReviewClient(r.Context(), serverID)
	if err != nil {
		writeHVProblem(w, http.StatusBadGateway, "agenthub_review_unavailable", "AgentHub could not authorize this action.", "", "")
		return false
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	provenance, err := client.Authorize(ctx, agenthub.Request{TenantID: identity.AgentHubTenantID, MachineID: identity.AgentHubMachineID,
		TicketID: review.AgentHubTicket, Tool: "shippin:infrastructure.guest_power", Command: infraPowerCommand(actionSHA, action), ActionSHA256: actionSHA})
	if err != nil {
		code := "agenthub_review_unavailable"
		status := http.StatusBadGateway
		if errors.Is(err, agenthub.ErrDenied) {
			code, status = "agenthub_review_denied", http.StatusConflict
		}
		writeHVProblem(w, status, code, "AgentHub did not authorize this guest action.", "", "")
		return false
	}
	if provenance.TicketSHA256 != review.TicketSHA256 {
		writeHVProblem(w, http.StatusConflict, "agenthub_review_stale", "The AgentHub ticket changed after Shippin submitted it for review.", "", "")
		return false
	}
	// Re-read after AgentHub authorization and open the exact endpoint snapshot
	// immediately before consuming the decision. Never call a client opened from
	// a different configuration revision than the one reviewed.
	liveEndpoint, err := a.powerEndpoint(r.Context(), tenant, serverID)
	if err != nil || infraEndpointSHA(liveEndpoint) != review.EndpointSHA256 {
		writeHVProblem(w, http.StatusConflict, "agenthub_review_stale", "The provider connection changed while AgentHub was authorizing the action.", "", "")
		return false
	}
	liveDriver, err := a.openDriver(liveEndpoint)
	if err != nil {
		infraErr(w, err)
		return false
	}
	liveGuest, err := liveDriver.Guest(r.Context(), guest)
	if err != nil {
		infraErr(w, err)
		return false
	}
	if !a.verifyInfraPowerTarget(w, r, c, liveGuest, guest, verb) {
		return false
	}
	proof, _ := json.Marshal(struct {
		agenthub.Provenance
		ActionSHA256 string `json:"action_sha256"`
	}{provenance, actionSHA})
	claimed, err := a.serverStore().ClaimPowerReview(r.Context(), review.ID, serverID, actionSHA, provenance.TicketSHA256, json.RawMessage(proof))
	if err != nil || !claimed {
		writeHVProblem(w, http.StatusConflict, "agenthub_review_consumed", "This review was already used or could not be recorded.", "", "")
		return false
	}
	g, callErr := a.tracked(r, c, serverID, guest, verb, func(ctx context.Context) (hv.Guest, error) { return runVerb(ctx, liveDriver, verb, guest) })
	state, failure := servers.OpSucceeded, ""
	if callErr != nil {
		state, _ = operationOutcome(callErr)
		if state == servers.OpUnknown {
			failure = "provider_outcome_unknown"
		} else {
			state, failure = "failed_execution", "provider_action_failed"
		}
	}
	if err := a.serverStore().FinishPowerReview(context.WithoutCancel(r.Context()), review.ID, state, failure); err != nil {
		infraReviewUnavailable(w)
		return false
	}
	if callErr != nil {
		infraErr(w, callErr)
		return false
	}
	writeJSON(w, g)
	return true
}

func (a *API) verifyInfraPowerTarget(w http.ResponseWriter, r *http.Request, c *caller, guestRecord hv.Guest, guestID, verb string) bool {
	if guestRecord.ID != guestID {
		writeHVProblem(w, http.StatusConflict, "guest_binding_mismatch", "The provider returned a different guest than requested.", "", "")
		return false
	}
	if verb == "start" || !isSelf(guestRecord) {
		return true
	}
	if r.URL.Query().Get("confirm") != "self" {
		writeHVProblem(w, http.StatusConflict, "self_guest", "This panel runs on that guest. Doing this takes the panel down; confirm it to go ahead.", "", "")
		return false
	}
	if !a.infraCan(r.Context(), c, "infra.self") {
		writeHVProblem(w, http.StatusForbidden, "self_guest", "Changing the guest this panel runs on needs the infra.self permission.", "", "")
		return false
	}
	return true
}

func infraPowerTicketMatches(ticket agenthub.TicketSummary, tenant, machine, ticketID, command, actionSHA, serverID, guest, verb, endpointSHA string) bool {
	return ticket.ID == ticketID && ticket.TenantID == tenant && ticket.Machine != nil && *ticket.Machine == machine &&
		ticket.Command == command && ticket.Fields["shippin_action_sha256"] == actionSHA &&
		ticket.Fields["shippin_agenthub_ticket_id"] == ticketID && ticket.Fields["shippin_system_id"] == serverID &&
		ticket.Fields["shippin_guest_id"] == guest && ticket.Fields["shippin_guest_verb"] == verb &&
		ticket.Fields["shippin_endpoint_config_sha256"] == endpointSHA
}

func (a *API) powerEndpoint(ctx context.Context, tenant, serverID string) (servers.Endpoint, error) {
	if tenant != "" {
		return a.visibleEndpoint(ctx, tenant, serverID)
	}
	return a.hvDir().Endpoint(ctx, serverID)
}

func infraEndpointSHA(ep servers.Endpoint) string {
	secret := sha256.Sum256(ep.SealedSecret)
	key := sha256.Sum256(ep.SealedKey)
	config := struct{ Kind, BaseURL, Node, TokenID, TokenEnv, CAFile, CertFile, KeyFile, TrustedCert, ClientCert, SecretSHA, ClientKeySHA string }{
		ep.Kind, ep.BaseURL, ep.Node, ep.TokenID, ep.TokenEnv, ep.CAFile, ep.CertFile, ep.KeyFile, ep.TrustedCert, ep.ClientCert,
		hex.EncodeToString(secret[:]), hex.EncodeToString(key[:])}
	b, _ := json.Marshal(config)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func infraPowerCommand(actionSHA string, action infraPowerAction) string {
	b, _ := json.Marshal(struct {
		Operation string `json:"operation"`
		ServerID  string `json:"server_id"`
		Guest     string `json:"guest_id"`
		Verb      string `json:"verb"`
		ConfigSHA string `json:"endpoint_config_sha256"`
		ActionSHA string `json:"action_sha256"`
	}{
		"shippin:infrastructure.guest_power", action.ServerID, action.GuestID, action.Verb, action.EndpointSHA, actionSHA})
	return string(b)
}

func validPowerVerb(verb string) error {
	switch verb {
	case "start", "stop", "shutdown", "reboot":
		return nil
	}
	return errors.New("invalid guest power verb")
}
func infraReviewUnavailable(w http.ResponseWriter) {
	writeHVProblem(w, http.StatusServiceUnavailable, "agenthub_review_unavailable", "The AgentHub guest review could not be recorded or verified.", "", "")
}
func (a *API) failInfraPowerReview(w http.ResponseWriter, r *http.Request, review servers.PowerReview, code string) {
	updated, err := a.serverStore().SetPowerReviewState(r.Context(), review.ID, "failed", "", code)
	if err != nil {
		infraReviewUnavailable(w)
		return
	}
	writeStatus(w, http.StatusBadGateway, map[string]any{"review": updated, "message": "AgentHub did not complete the review request. Retry with the same idempotency key to resume."})
}
