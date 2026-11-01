package catalog

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"

	"shippin.cloud/console/internal/auth"
)

// Handler serves the catalog API; Routes lists each endpoint with the permission that gates it.
// Can answers field-level questions (prices, contacts, notes) for the current caller; nil (the
// loopback demo binary) allows everything.
type Handler struct {
	S   *Store
	Can func(r *http.Request, perm string) bool
}

// Route is mounted by the caller (the console gates it by Perm; the demo binary does not).
type Route struct {
	Pattern string
	Perm    string
	H       func(w http.ResponseWriter, r *http.Request, actor string)
}

// Granular permissions (internal/api/perm_catalog.go): deny by default, Owner preset or a
// direct grant only. Route gates are enforced by the console; field gates here.
const (
	PermPlansRead      = "catalog.plans.read"
	PermPricesRead     = "catalog.prices.read"
	PermPlansWrite     = "catalog.plans.write"
	PermDefaultsWrite  = "config.defaults.write"
	PermCustomersRead  = "customers.read"
	PermContactsRead   = "customers.contacts.read"
	PermNotesRead      = "customers.notes.read"
	PermCustomersWrite = "customers.write"
	PermServicesWrite  = "customers.services.write"
	PermEffectiveRead  = "config.effective.read"
	PermIntentCreate   = "provision.intent.create"
)

func (h *Handler) Routes() []Route {
	return []Route{
		{"GET /v1/operator/catalog/plans", PermPlansRead, h.listPlans},
		{"POST /v1/operator/catalog/plans", PermPlansWrite, h.createPlan},
		{"GET /v1/operator/catalog/plans/{id}", PermPlansRead, h.getPlan},
		{"PUT /v1/operator/catalog/plans/{id}", PermPlansWrite, h.updatePlan},
		{"DELETE /v1/operator/catalog/plans/{id}", PermPlansWrite, h.deletePlan},
		{"GET /v1/operator/catalog/services", PermPlansRead, h.listServices},
		{"POST /v1/operator/catalog/services", PermPlansWrite, h.createService},
		{"GET /v1/operator/catalog/services/{id}", PermPlansRead, h.getService},
		{"PUT /v1/operator/catalog/services/{id}", PermPlansWrite, h.updateService},
		{"DELETE /v1/operator/catalog/services/{id}", PermPlansWrite, h.deleteService},
		{"GET /v1/operator/catalog/plan-services", PermPlansRead, h.listPlanServices},
		{"PUT /v1/operator/catalog/plan-services/{plan}/{service}", PermPlansWrite, h.putPlanService},
		{"DELETE /v1/operator/catalog/plan-services/{plan}/{service}", PermPlansWrite, h.deletePlanService},
		{"GET /v1/operator/catalog/defaults", PermPlansRead, h.listDefaults},
		{"PUT /v1/operator/catalog/defaults", PermDefaultsWrite, h.putDefault},
		{"DELETE /v1/operator/catalog/defaults/{layer}/{key}", PermDefaultsWrite, h.deleteDefault},
		{"GET /v1/operator/customers", PermCustomersRead, h.listCustomers},
		{"POST /v1/operator/customers", PermCustomersWrite, h.createCustomer},
		{"GET /v1/operator/customers/{id}", PermCustomersRead, h.getCustomer},
		{"PUT /v1/operator/customers/{id}", PermCustomersWrite, h.updateCustomer},
		{"DELETE /v1/operator/customers/{id}", PermCustomersWrite, h.deleteCustomer},
		{"GET /v1/operator/customers/{id}/services", PermCustomersRead, h.listTenantServices},
		{"PUT /v1/operator/customers/{id}/services/{service}", PermServicesWrite, h.putTenantService},
		{"DELETE /v1/operator/customers/{id}/services/{service}", PermServicesWrite, h.deleteTenantService},
		{"GET /v1/operator/customers/{id}/effective-config", PermEffectiveRead, h.effective},
		{"GET /v1/operator/customers/{id}/provision-intents", PermCustomersRead, h.listIntents},
		{"POST /v1/operator/customers/{id}/provision-intent", PermIntentCreate, h.provisionIntent},
	}
}

var slug = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,39}$`)

func validLock(l string) bool {
	return l == LockLocked || l == LockOverridable || l == LockMergeable || l == LockBounded
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	if raw, ok := v.(json.RawMessage); ok {
		_, _ = w.Write(raw)
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(dst); err != nil {
		auth.Problem(w, http.StatusBadRequest, "The request body isn't valid JSON.")
		return false
	}
	return true
}

// fail maps store errors to problem responses without leaking SQL details.
func fail(w http.ResponseWriter, err error, what string) {
	switch {
	case errors.Is(err, ErrNotFound):
		auth.Problem(w, http.StatusNotFound, what+" not found.")
	case err != nil && isConflict(err):
		auth.Problem(w, http.StatusConflict, what+" already exists or is still referenced.")
	default:
		auth.Problem(w, http.StatusInternalServerError, "Couldn't save "+what+". Try again.")
	}
}

// items / one write a list or object, without the fields named in hide (field-level permissions).
func items(w http.ResponseWriter, raw json.RawMessage, err error, what string, hide ...string) {
	if err == nil {
		raw, err = strip(raw, hide)
	}
	if err != nil {
		fail(w, err, what)
		return
	}
	writeJSON(w, http.StatusOK, json.RawMessage(`{"items":`+string(raw)+`}`))
}

func one(w http.ResponseWriter, raw json.RawMessage, err error, what string, hide ...string) {
	if err == nil {
		raw, err = strip(raw, hide)
	}
	if err != nil {
		fail(w, err, what)
		return
	}
	writeJSON(w, http.StatusOK, raw)
}
