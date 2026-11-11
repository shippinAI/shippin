package catalog

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/auth"
)

// Contact and Endpoint shapes are validated so the UI can rely on them.
type contact struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type endpoint struct {
	Kind string `json:"kind"`
	Host string `json:"host"`
	Note string `json:"note"`
}

func (h *Handler) listCustomers(w http.ResponseWriter, r *http.Request, _ string) {
	raw, err := h.S.Customers(r.Context())
	items(w, raw, err, "Customers", h.hiddenCustomerFields(r)...)
}

func (h *Handler) getCustomer(w http.ResponseWriter, r *http.Request, _ string) {
	raw, err := h.S.Customer(r.Context(), r.PathValue("id"))
	one(w, raw, err, "Customer", h.hiddenCustomerFields(r)...)
}

func (h *Handler) readCustomer(w http.ResponseWriter, r *http.Request) (Customer, bool) {
	var c Customer
	if !decodeBody(w, r, &c) {
		return c, false
	}
	c.Name = strings.TrimSpace(c.Name)
	var cs []contact
	var es []endpoint
	if len(c.Contacts) == 0 {
		c.Contacts = json.RawMessage(`[]`)
	}
	if len(c.Endpoints) == 0 {
		c.Endpoints = json.RawMessage(`[]`)
	}
	if c.Name == "" || len(c.Name) > 120 || json.Unmarshal(c.Contacts, &cs) != nil || json.Unmarshal(c.Endpoints, &es) != nil {
		auth.Problem(w, http.StatusBadRequest, "A customer needs a name (up to 120 chars); contacts and endpoints must be lists.")
		return c, false
	}
	for _, x := range cs {
		if x.Email != "" && !strings.Contains(x.Email, "@") {
			auth.Problem(w, http.StatusBadRequest, "Contact e-mail "+x.Email+" doesn't look like an address.")
			return c, false
		}
	}
	if !h.S.PlanExists(r.Context(), c.Plan) {
		auth.Problem(w, http.StatusBadRequest, "Pick a plan that exists in Plans & Tiers.")
		return c, false
	}
	if c.Coverage != nil && *c.Coverage == "" {
		c.Coverage = nil
	}
	if c.Coverage != nil && !h.S.CoverageExists(r.Context(), *c.Coverage) {
		auth.Problem(w, http.StatusBadRequest, "Pick a coverage tier that exists, or leave it empty to use the plan's.")
		return c, false
	}
	return c, true
}

func (h *Handler) createCustomer(w http.ResponseWriter, r *http.Request, _ string) {
	c, ok := h.readCustomer(w, r)
	if !ok {
		return
	}
	if setsHidden(c, h.hiddenCustomerFields(r)) {
		auth.Problem(w, http.StatusForbidden, "Contacts and endpoints need customers.contacts.read; notes need customers.notes.read.")
		return
	}
	id, _ := uuid.NewV7()
	c.ID, c.Status = id.String(), "draft"
	if err := h.S.CreateCustomer(r.Context(), c); err != nil {
		fail(w, err, "Customer")
		return
	}
	raw, err := h.S.Customer(r.Context(), c.ID)
	if err != nil {
		fail(w, err, "Customer")
		return
	}
	raw, err = strip(raw, h.hiddenCustomerFields(r))
	if err != nil {
		fail(w, err, "Customer")
		return
	}
	writeJSON(w, http.StatusCreated, raw)
}

func (h *Handler) updateCustomer(w http.ResponseWriter, r *http.Request, _ string) {
	c, ok := h.readCustomer(w, r)
	if !ok {
		return
	}
	c.ID = r.PathValue("id")
	if hide := h.hiddenCustomerFields(r); len(hide) > 0 {
		stored, err := h.S.CustomerRow(r.Context(), c.ID)
		if err != nil {
			fail(w, err, "Customer")
			return
		}
		keepHidden(&c, stored, hide)
	}
	if err := h.S.UpdateCustomer(r.Context(), c); err != nil {
		fail(w, err, "Customer")
		return
	}
	raw, err := h.S.Customer(r.Context(), c.ID)
	one(w, raw, err, "Customer", h.hiddenCustomerFields(r)...)
}

func (h *Handler) deleteCustomer(w http.ResponseWriter, r *http.Request, _ string) {
	if err := h.S.DeleteCustomer(r.Context(), r.PathValue("id")); err != nil {
		auth.Problem(w, http.StatusConflict, "Only unlinked draft customers without workspaces can be deleted here; use Tenants for live ones.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
