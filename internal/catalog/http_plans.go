package catalog

import (
	"encoding/json"
	"net/http"
	"strconv"

	"shippin.cloud/console/internal/auth"
)

func (h *Handler) listPlans(w http.ResponseWriter, r *http.Request, _ string) {
	raw, err := h.S.Plans(r.Context())
	items(w, raw, err, "Plans", h.hiddenPlanFields(r)...)
}

func (h *Handler) getPlan(w http.ResponseWriter, r *http.Request, _ string) {
	raw, err := h.S.Plan(r.Context(), r.PathValue("id"))
	one(w, raw, err, "Plan", h.hiddenPlanFields(r)...)
}

func (h *Handler) readPlan(w http.ResponseWriter, r *http.Request) (Plan, bool) {
	p := Plan{Active: true, Public: true}
	if !decodeBody(w, r, &p) {
		return p, false
	}
	if id := r.PathValue("id"); id != "" {
		p.ID = id
	}
	if !slug.MatchString(p.ID) || p.Label == "" || p.PriceCents < 0 {
		auth.Problem(w, http.StatusBadRequest, "A plan needs a lowercase id (2-40 chars), a label and a non-negative price.")
		return p, false
	}
	return p, true
}

func (h *Handler) createPlan(w http.ResponseWriter, r *http.Request, _ string) {
	if p, ok := h.readPlan(w, r); ok {
		if p.PriceCents != 0 && !h.can(r, PermPricesRead) {
			auth.Problem(w, http.StatusForbidden, "Setting a price needs catalog.prices.read.")
			return
		}
		if err := h.S.CreatePlan(r.Context(), p); err != nil {
			fail(w, err, "Plan")
			return
		}
		h.writePlan(w, r, http.StatusCreated, p)
	}
}

// updatePlan keeps the stored price when the caller can't see prices (catalog.prices.read).
func (h *Handler) updatePlan(w http.ResponseWriter, r *http.Request, _ string) {
	if p, ok := h.readPlan(w, r); ok {
		if !h.can(r, PermPricesRead) {
			price, err := h.S.PlanPrice(r.Context(), p.ID)
			if err != nil {
				fail(w, err, "Plan")
				return
			}
			p.PriceCents = price
		}
		if err := h.S.UpdatePlan(r.Context(), p); err != nil {
			fail(w, err, "Plan")
			return
		}
		h.writePlan(w, r, http.StatusOK, p)
	}
}

func (h *Handler) writePlan(w http.ResponseWriter, r *http.Request, code int, p Plan) {
	b, _ := json.Marshal(p)
	raw, err := strip(b, h.hiddenPlanFields(r))
	if err != nil {
		fail(w, err, "Plan")
		return
	}
	writeJSON(w, code, json.RawMessage(raw))
}

func (h *Handler) deletePlan(w http.ResponseWriter, r *http.Request, _ string) {
	n, err := h.S.DeletePlan(r.Context(), r.PathValue("id"))
	if n > 0 {
		auth.Problem(w, http.StatusConflict, strconv.Itoa(n)+" customer(s) are still on this plan. Move them first.")
		return
	}
	if err != nil {
		fail(w, err, "Plan")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listPlanServices(w http.ResponseWriter, r *http.Request, _ string) {
	raw, err := h.S.PlanServices(r.Context())
	items(w, raw, err, "Plan services")
}

func (h *Handler) putPlanService(w http.ResponseWriter, r *http.Request, _ string) {
	p := PlanService{Enabled: true, Lock: LockBounded}
	if !decodeBody(w, r, &p) {
		return
	}
	p.PlanID, p.ServiceID = r.PathValue("plan"), r.PathValue("service")
	if len(p.Limits) == 0 {
		p.Limits = json.RawMessage(`{}`)
	}
	var m map[string]any
	if !validLock(p.Lock) || json.Unmarshal(p.Limits, &m) != nil {
		auth.Problem(w, http.StatusBadRequest, "Lock must be locked, overridable, mergeable or bounded; limits must be an object.")
		return
	}
	if err := h.S.PutPlanService(r.Context(), p); err != nil {
		fail(w, err, "Plan service")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (h *Handler) deletePlanService(w http.ResponseWriter, r *http.Request, _ string) {
	if err := h.S.DeletePlanService(r.Context(), r.PathValue("plan"), r.PathValue("service")); err != nil {
		fail(w, err, "Plan service")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
