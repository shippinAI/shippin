package catalog

import (
	"encoding/json"
	"net/http"
	"strconv"

	"shippin.cloud/console/internal/auth"
)

func (h *Handler) hiddenCoverageFields(r *http.Request) []string {
	if h.can(r, PermPricesRead) {
		return nil
	}
	return []string{"price_cents", "spend_percent"}
}

func (h *Handler) listCoverage(w http.ResponseWriter, r *http.Request, _ string) {
	raw, err := h.S.CoverageTiers(r.Context())
	items(w, raw, err, "Coverage tiers", h.hiddenCoverageFields(r)...)
}

func (h *Handler) getCoverage(w http.ResponseWriter, r *http.Request, _ string) {
	raw, err := h.S.CoverageTier(r.Context(), r.PathValue("id"))
	one(w, raw, err, "Coverage tier", h.hiddenCoverageFields(r)...)
}

// readCoverage decodes and validates a tier; the reasons name the field so the form can say what is wrong.
func (h *Handler) readCoverage(w http.ResponseWriter, r *http.Request) (CoverageTier, bool) {
	c := CoverageTier{SignIn: "password", SupportHours: "business", Channels: []string{"docs", "email"}, Operations: "self", Reviews: "none",
		Consulting: "quoted", CustomDeployments: "quoted", RemoteOperations: "none", TeamTraining: "quoted", Public: true, Active: true}
	if !decodeBody(w, r, &c) {
		return c, false
	}
	if id := r.PathValue("id"); id != "" {
		c.ID = id
	}
	bad := func(m string) (CoverageTier, bool) { auth.Problem(w, http.StatusBadRequest, m); return c, false }
	switch {
	case !slug.MatchString(c.ID) || c.Label == "" || len(c.Label) > 80:
		return bad("A coverage tier needs a lowercase id (2-40 chars) and a label of up to 80 characters.")
	case !oneOf(c.SignIn, SignInRules):
		return bad("Sign-in must be password, second_factor or identity_provider.")
	case !oneOf(c.SupportHours, SupportHours):
		return bad("Support hours must be business, critical_always or always.")
	case !oneOf(c.Operations, OperationModes):
		return bad("Operations must be self, assisted or managed.")
	case !oneOf(c.Reviews, ReviewCadences):
		return bad("Reviews must be none, quarterly or monthly.")
	case !oneOf(c.Consulting, ServiceTerms) || !oneOf(c.CustomDeployments, ServiceTerms) || !oneOf(c.RemoteOperations, ServiceTerms) || !oneOf(c.TeamTraining, ServiceTerms):
		return bad("Consulting, custom deployments, remote operations and team training must each be none, quoted or included.")
	case c.TrainingHours < 0 || c.TrainingHours > 10000:
		return bad("Training hours must be between 0 and 10000 a year.")
	case c.PriceCents < 0 || c.SpendPercent < 0 || c.SpendPercent > 100:
		return bad("The price must not be negative, and the share of plan charges must be between 0 and 100 percent.")
	case c.UptimeTarget != nil && (*c.UptimeTarget <= 0 || *c.UptimeTarget >= 100):
		return bad("An uptime target is a percentage below 100, such as 99.9.")
	}
	for _, m := range []*int{c.CriticalMinutes, c.HighMinutes, c.NormalMinutes} {
		if m != nil && *m <= 0 {
			return bad("A response target is a number of minutes above zero, or empty for none.")
		}
	}
	if c.Channels == nil {
		c.Channels = []string{}
	}
	for _, ch := range c.Channels {
		if !oneOf(ch, SupportChannels) {
			return bad("Channels are docs, community, email, chat, phone and video.")
		}
	}
	return c, true
}

func (h *Handler) createCoverage(w http.ResponseWriter, r *http.Request, _ string) {
	c, ok := h.readCoverage(w, r)
	if !ok {
		return
	}
	if (c.PriceCents != 0 || c.SpendPercent != 0) && !h.can(r, PermPricesRead) {
		auth.Problem(w, http.StatusForbidden, "Setting a price needs catalog.prices.read.")
		return
	}
	if err := h.S.CreateCoverage(r.Context(), c); err != nil {
		fail(w, err, "Coverage tier")
		return
	}
	h.writeCoverage(w, r, http.StatusCreated, c.ID)
}

// updateCoverage keeps the stored price when the caller can't see prices (catalog.prices.read).
func (h *Handler) updateCoverage(w http.ResponseWriter, r *http.Request, _ string) {
	c, ok := h.readCoverage(w, r)
	if !ok {
		return
	}
	if !h.can(r, PermPricesRead) {
		cents, pct, err := h.S.CoveragePrice(r.Context(), c.ID)
		if err != nil {
			fail(w, err, "Coverage tier")
			return
		}
		c.PriceCents, c.SpendPercent = cents, pct
	}
	if err := h.S.UpdateCoverage(r.Context(), c); err != nil {
		fail(w, err, "Coverage tier")
		return
	}
	h.writeCoverage(w, r, http.StatusOK, c.ID)
}

func (h *Handler) writeCoverage(w http.ResponseWriter, r *http.Request, code int, id string) {
	raw, err := h.S.CoverageTier(r.Context(), id)
	if err == nil {
		raw, err = strip(raw, h.hiddenCoverageFields(r))
	}
	if err != nil {
		fail(w, err, "Coverage tier")
		return
	}
	writeJSON(w, code, json.RawMessage(raw))
}

func (h *Handler) deleteCoverage(w http.ResponseWriter, r *http.Request, _ string) {
	n, err := h.S.DeleteCoverage(r.Context(), r.PathValue("id"))
	if n > 0 {
		auth.Problem(w, http.StatusConflict, strconv.Itoa(n)+" plan(s) include this coverage. Give them another tier first.")
		return
	}
	if err != nil {
		fail(w, err, "Coverage tier")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
