package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/catalog"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/deployments"
)

func (a *API) deploymentStore() *deployments.Store {
	if a.deployments != nil {
		return a.deployments
	}
	return deployments.NewStore(a.db.Pool)
}

func (a *API) deploymentRoutes(mux Router) {
	mux.HandleFunc("GET /v1/tenants/{tenantId}/deployable-services", a.flow.Require(a.listDeployableServices))
	mux.HandleFunc("GET /v1/tenants/{tenantId}/deployment-targets", a.flow.Require(a.listDeploymentTargets))
	mux.HandleFunc("GET /v1/tenants/{tenantId}/deployment-targets/{systemId}/storage", a.flow.Require(a.listDeploymentTargetStorage))
	mux.HandleFunc("GET /v1/tenants/{tenantId}/deployments", a.flow.Require(a.listDeployments))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/deployments", a.flow.Require(a.createDeployment))
	mux.HandleFunc("GET /v1/tenants/{tenantId}/deployments/{deploymentId}", a.flow.Require(a.getDeployment))
	mux.HandleFunc("PATCH /v1/tenants/{tenantId}/deployments/{deploymentId}", a.flow.Require(a.updateDeployment))
	mux.HandleFunc("DELETE /v1/tenants/{tenantId}/deployments/{deploymentId}", a.flow.Require(a.deleteDeployment))
	mux.HandleFunc("GET /v1/tenants/{tenantId}/deployments/{deploymentId}/events", a.flow.Require(a.listDeploymentEvents))
	mux.HandleFunc("GET /v1/tenants/{tenantId}/deployments/{deploymentId}/review", a.flow.Require(a.getDeploymentReview))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/deployments/{deploymentId}/review", a.flow.Require(a.prepareDeploymentReview))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/deployments/{deploymentId}/review/authorize", a.flow.Require(a.authorizeDeploymentReview))
}

func (a *API) listDeployableServices(w http.ResponseWriter, r *http.Request) {
	_, _, ok := a.deploymentTenantCtx(w, r)
	if !ok {
		return
	}
	items, err := (&catalog.Store{Pool: a.db.Pool}).DeployableServices(r.Context())
	if err != nil {
		deploymentUnavailable(w)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func (a *API) listDeploymentTargets(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.deploymentTenantCtx(w, r)
	if !ok {
		return
	}
	items, err := a.deploymentStore().Targets(r.Context(), t.ID)
	if err != nil {
		deploymentUnavailable(w)
		return
	}
	ui := a.uiConfig(r.Context())
	for i := range items {
		items[i].ProviderLabel = configuredProviderLabel(ui, items[i].Provider)
	}
	writeJSON(w, map[string]any{"items": items, "can_request": c.canDecide(t)})
}

// listDeploymentTargetStorage exposes live read-only storage inventory only
// after confirming that the requested host is actively assigned to this tenant.
func (a *API) listDeploymentTargetStorage(w http.ResponseWriter, r *http.Request) {
	_, t, ok := a.deploymentTenantCtx(w, r)
	if !ok {
		return
	}
	targets, err := a.deploymentStore().Targets(r.Context(), t.ID)
	if err != nil {
		deploymentUnavailable(w)
		return
	}
	var target *deployments.Target
	for i := range targets {
		if targets[i].ID == r.PathValue("systemId") {
			target = &targets[i]
			break
		}
	}
	if target == nil {
		auth.Problem(w, http.StatusNotFound, "Deployment target not found.")
		return
	}
	if target.State != "active" {
		auth.Problem(w, http.StatusConflict, "Storage inventory requires an active, connected target.")
		return
	}
	if target.Provider != deployments.ProviderIncus {
		auth.Problem(w, http.StatusConflict, "This runtime does not support storage selection for the target provider.")
		return
	}
	driver, err := a.driver(r.Context(), target.ID)
	if err != nil {
		deploymentUnavailable(w)
		return
	}
	items, err := driver.Storage(r.Context())
	if err != nil {
		deploymentUnavailable(w)
		return
	}
	writeJSON(w, map[string]any{"items": items})
}

func configuredProviderLabel(ui config.UI, provider deployments.Provider) string {
	if presentation, ok := ui.Provider(string(provider)); ok && presentation.Label != "" {
		return presentation.Label
	}
	return string(provider)
}

func (a *API) decorateDeployment(ctx context.Context, item *deployments.Deployment) {
	if item != nil {
		item.TargetProviderLabel = configuredProviderLabel(a.uiConfig(ctx), item.TargetProvider)
	}
}

func (a *API) deploymentTenantCtx(w http.ResponseWriter, r *http.Request) (*caller, *tenantView, bool) {
	c, ok := a.resolve(w, r)
	if !ok {
		return nil, nil, false
	}
	t, ok := c.tenant(w, r.PathValue("tenantId"))
	return c, t, ok
}

func (a *API) listDeployments(w http.ResponseWriter, r *http.Request) {
	_, t, ok := a.deploymentTenantCtx(w, r)
	if !ok {
		return
	}
	items, err := a.deploymentStore().List(r.Context(), t.ID, queryLimit(r, "limit", 100, 200))
	if err != nil {
		deploymentUnavailable(w)
		return
	}
	ui := a.uiConfig(r.Context())
	for i := range items {
		items[i].TargetProviderLabel = configuredProviderLabel(ui, items[i].TargetProvider)
	}
	writeJSON(w, map[string]any{"items": items, "page": map[string]any{"next_page_token": nil, "total_estimate": len(items)}})
}

func (a *API) getDeployment(w http.ResponseWriter, r *http.Request) {
	_, t, ok := a.deploymentTenantCtx(w, r)
	if !ok {
		return
	}
	item, err := a.deploymentStore().Get(r.Context(), t.ID, r.PathValue("deploymentId"))
	if err != nil {
		writeDeploymentError(w, err)
		return
	}
	a.decorateDeployment(r.Context(), item)
	w.Header().Set("ETag", deploymentETag(item.Generation))
	writeJSON(w, item)
}

func (a *API) createDeployment(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.deploymentTenantCtx(w, r)
	if !ok {
		return
	}
	if !c.canDecide(t) {
		auth.Problem(w, http.StatusForbidden, "A tenant owner or admin is required to request a deployment.")
		return
	}
	key, ok := deploymentIdempotencyKey(w, r)
	if !ok {
		return
	}
	a.idempotent(w, r, t.ID+":"+c.id.Sub, func(w http.ResponseWriter, r *http.Request) {
		var input deployments.CreateInput
		if !decodeDeploymentBody(w, r, &input) {
			return
		}
		if err := a.validateDeploymentStorageTarget(r.Context(), t.ID, input.TargetSystemID, input.TargetConfig); err != nil {
			switch {
			case errors.Is(err, deployments.ErrTargetUnavailable):
				writeDeploymentError(w, err)
			case errors.Is(err, errDeploymentStoragePoolUnavailable):
				auth.Problem(w, http.StatusConflict, "The selected storage pool is no longer available on this target. Refresh the pool list and retry.")
			default:
				deploymentUnavailable(w)
			}
			return
		}
		input.IdempotencyKey = key
		item, created, err := a.deploymentStore().Create(r.Context(), t.ID, c.id.Sub, input)
		if err != nil {
			writeDeploymentError(w, err)
			return
		}
		a.decorateDeployment(r.Context(), item)
		w.Header().Set("ETag", deploymentETag(item.Generation))
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		writeStatus(w, status, item)
	})
}

func (a *API) updateDeployment(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.deploymentTenantCtx(w, r)
	if !ok {
		return
	}
	if !c.canDecide(t) {
		auth.Problem(w, http.StatusForbidden, "A tenant owner or admin is required to change deployment intent.")
		return
	}
	key, ok := deploymentIdempotencyKey(w, r)
	if !ok {
		return
	}
	expected, ok := deploymentIfMatch(w, r)
	if !ok {
		return
	}
	a.idempotent(w, r, t.ID+":"+c.id.Sub, func(w http.ResponseWriter, r *http.Request) {
		var input deployments.UpdateInput
		if !decodeDeploymentBody(w, r, &input) {
			return
		}
		input.IdempotencyKey = key
		item, _, err := a.deploymentStore().Update(r.Context(), t.ID, r.PathValue("deploymentId"), c.id.Sub, expected, input)
		if err != nil {
			writeDeploymentError(w, err)
			return
		}
		a.decorateDeployment(r.Context(), item)
		w.Header().Set("ETag", deploymentETag(item.Generation))
		writeJSON(w, item)
	})
}

// Deletion is a desired-state transition. A provider resource is not reported absent until a
// separately authorized reconciler observes that result.
func (a *API) deleteDeployment(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.deploymentTenantCtx(w, r)
	if !ok {
		return
	}
	if !c.canDecide(t) {
		auth.Problem(w, http.StatusForbidden, "A tenant owner or admin is required to change deployment intent.")
		return
	}
	key, ok := deploymentIdempotencyKey(w, r)
	if !ok {
		return
	}
	expected, ok := deploymentIfMatch(w, r)
	if !ok {
		return
	}
	a.idempotent(w, r, t.ID+":"+c.id.Sub, func(w http.ResponseWriter, r *http.Request) {
		item, _, err := a.deploymentStore().Update(r.Context(), t.ID, r.PathValue("deploymentId"), c.id.Sub, expected,
			deployments.UpdateInput{DesiredState: deployments.DesiredAbsent, IdempotencyKey: key})
		if err != nil {
			writeDeploymentError(w, err)
			return
		}
		a.decorateDeployment(r.Context(), item)
		w.Header().Set("ETag", deploymentETag(item.Generation))
		writeJSON(w, item)
	})
}

func (a *API) listDeploymentEvents(w http.ResponseWriter, r *http.Request) {
	_, t, ok := a.deploymentTenantCtx(w, r)
	if !ok {
		return
	}
	items, err := a.deploymentStore().Events(r.Context(), t.ID, r.PathValue("deploymentId"), queryLimit(r, "limit", 100, 500))
	if err != nil {
		writeDeploymentError(w, err)
		return
	}
	writeJSON(w, map[string]any{"items": items, "page": map[string]any{"next_page_token": nil, "total_estimate": len(items)}})
}

func decodeDeploymentBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "request body too large") {
			status = http.StatusRequestEntityTooLarge
		}
		auth.Problem(w, status, "Send a valid deployment JSON body.")
		return false
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		auth.Problem(w, http.StatusBadRequest, "Send exactly one deployment JSON value.")
		return false
	}
	return true
}

func deploymentIdempotencyKey(w http.ResponseWriter, r *http.Request) (string, bool) {
	key := r.Header.Get("Idempotency-Key")
	if _, err := uuid.Parse(key); err != nil {
		auth.Problem(w, http.StatusBadRequest, "Idempotency-Key must be a UUID.")
		return "", false
	}
	return key, true
}

func deploymentIfMatch(w http.ResponseWriter, r *http.Request) (int64, bool) {
	value := strings.TrimSpace(r.Header.Get("If-Match"))
	if len(value) < 3 || value[0] != '"' || value[len(value)-1] != '"' {
		auth.Problem(w, http.StatusPreconditionRequired, "Send the current deployment ETag in If-Match.")
		return 0, false
	}
	generation, err := strconv.ParseInt(value[1:len(value)-1], 10, 64)
	if err != nil || generation < 1 {
		auth.Problem(w, http.StatusBadRequest, "If-Match must contain a deployment generation ETag.")
		return 0, false
	}
	return generation, true
}

func deploymentETag(generation int64) string { return `"` + strconv.FormatInt(generation, 10) + `"` }

func writeDeploymentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, deployments.ErrNotFound):
		auth.Problem(w, http.StatusNotFound, "Deployment not found.")
	case errors.Is(err, deployments.ErrIdempotencyConflict):
		auth.Problem(w, http.StatusUnprocessableEntity, "This Idempotency-Key was already used for a different deployment request.")
	case errors.Is(err, deployments.ErrGenerationConflict):
		auth.Problem(w, http.StatusPreconditionFailed, "The deployment changed. Reload it and retry with the current ETag.")
	case errors.Is(err, deployments.ErrDefinitionUnavailable):
		auth.Problem(w, http.StatusConflict, "This service does not have an active deployable definition.")
	case errors.Is(err, deployments.ErrTargetUnavailable):
		auth.Problem(w, http.StatusConflict, "The target system is unavailable or is not assigned to this tenant.")
	case errors.Is(err, deployments.ErrInvalidRequest), errors.Is(err, deployments.ErrInvalidConfig):
		auth.Problem(w, http.StatusUnprocessableEntity, "The deployment request or configuration is invalid.")
	default:
		deploymentUnavailable(w)
	}
}

func deploymentUnavailable(w http.ResponseWriter) {
	auth.Problem(w, http.StatusServiceUnavailable, "Deployment state is unavailable right now.")
}

func queryLimit(r *http.Request, name string, fallback, max int) int {
	value := r.URL.Query().Get(name)
	if value == "" {
		return fallback
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > max {
		return fallback
	}
	return limit
}
