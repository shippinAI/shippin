package catalog

import (
	"encoding/json"
	"net/http"
	"strings"

	"shippin.cloud/console/internal/auth"
)

func (h *Handler) listTenantServices(w http.ResponseWriter, r *http.Request, _ string) {
	ts, err := h.S.TenantServices(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, err, "Tenant services")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": ts})
}

func (h *Handler) putTenantService(w http.ResponseWriter, r *http.Request, _ string) {
	var t TenantService
	if !decodeBody(w, r, &t) {
		return
	}
	t.TenantID, t.ServiceID = r.PathValue("id"), r.PathValue("service")
	if len(t.Overrides) == 0 {
		t.Overrides = json.RawMessage(`{}`)
	}
	var m map[string]any
	if json.Unmarshal(t.Overrides, &m) != nil {
		auth.Problem(w, http.StatusBadRequest, "Overrides must be a JSON object of service settings.")
		return
	}
	if _, err := h.S.CustomerPlan(r.Context(), t.TenantID); err != nil {
		fail(w, err, "Customer")
		return
	}
	if err := h.S.PutTenantService(r.Context(), t); err != nil {
		fail(w, err, "Tenant service")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *Handler) deleteTenantService(w http.ResponseWriter, r *http.Request, _ string) {
	if err := h.S.DeleteTenantService(r.Context(), r.PathValue("id"), r.PathValue("service")); err != nil {
		fail(w, err, "Tenant service")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) effective(w http.ResponseWriter, r *http.Request, _ string) {
	eff, err := h.S.Effective(r.Context(), r.PathValue("id"), r.URL.Query().Get("workspace"))
	if err != nil {
		fail(w, err, "Customer")
		return
	}
	writeJSON(w, http.StatusOK, eff)
}

func (h *Handler) listIntents(w http.ResponseWriter, r *http.Request, _ string) {
	raw, err := h.S.Intents(r.Context(), r.PathValue("id"))
	items(w, raw, err, "Provision intents")
}

// provisionIntent records desired state only; Gold tickets apply it later.
func (h *Handler) provisionIntent(w http.ResponseWriter, r *http.Request, actor string) {
	var body struct {
		Note string `json:"note"`
	}
	if r.ContentLength > 0 && !decodeBody(w, r, &body) {
		return
	}
	raw, err := h.S.RecordIntent(r.Context(), r.PathValue("id"), actor, strings.TrimSpace(body.Note))
	if err != nil {
		fail(w, err, "Customer")
		return
	}
	writeJSON(w, http.StatusAccepted, raw)
}
