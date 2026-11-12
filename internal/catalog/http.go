package catalog

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/mcpl"
)

// Handler serves the catalog API; Routes lists each endpoint with the permission that gates it.
// Can answers field-level questions (prices, contacts, notes) for the current caller; nil (the
// loopback demo binary) allows everything.
type Handler struct {
	S   *Store
	Can func(r *http.Request, perm string) bool
}

// Route is mounted by the caller (the console gates it by Perm; the demo binary does not). ID and
// Summary describe it as an MCPL action (docs/mcpl.md); its risk follows the HTTP method.
type Route struct {
	Pattern string
	Perm    string
	H       func(w http.ResponseWriter, r *http.Request, actor string)
	ID      string
	Summary string
}

// Action is the route as an MCPL action. Catalog writes take no reason yet.
func (rt Route) Action() mcpl.Action {
	return mcpl.Action{ID: rt.ID, Route: rt.Pattern, Access: mcpl.Staff, Perm: rt.Perm, Risk: mcpl.RiskOf(rt.Pattern), Summary: rt.Summary}
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
		{"GET /v1/operator/catalog/plans", PermPlansRead, h.listPlans, "shippin.tenancy.plan.list", "List the plans customers can be on."},
		{"POST /v1/operator/catalog/plans", PermPlansWrite, h.createPlan, "shippin.tenancy.plan.create", "Create a plan."},
		{"GET /v1/operator/catalog/plans/{id}", PermPlansRead, h.getPlan, "shippin.tenancy.plan.get", "Read one plan."},
		{"PUT /v1/operator/catalog/plans/{id}", PermPlansWrite, h.updatePlan, "shippin.tenancy.plan.update", "Change a plan's label, price, limits or coverage."},
		{"DELETE /v1/operator/catalog/plans/{id}", PermPlansWrite, h.deletePlan, "shippin.tenancy.plan.delete", "Remove a plan no customer is on."},
		{"GET /v1/operator/catalog/coverage", PermPlansRead, h.listCoverage, "shippin.tenancy.coverage.list", "List the coverage tiers."},
		{"POST /v1/operator/catalog/coverage", PermPlansWrite, h.createCoverage, "shippin.tenancy.coverage.create", "Create a coverage tier."},
		{"GET /v1/operator/catalog/coverage/{id}", PermPlansRead, h.getCoverage, "shippin.tenancy.coverage.get", "Read one coverage tier."},
		{"PUT /v1/operator/catalog/coverage/{id}", PermPlansWrite, h.updateCoverage, "shippin.tenancy.coverage.update", "Change a coverage tier's support, services or sign-in rule."},
		{"DELETE /v1/operator/catalog/coverage/{id}", PermPlansWrite, h.deleteCoverage, "shippin.tenancy.coverage.delete", "Remove a coverage tier no plan includes."},
		{"GET /v1/operator/catalog/services", PermPlansRead, h.listServices, "shippin.services.catalog_service.list", "List the services plans can include."},
		{"POST /v1/operator/catalog/services", PermPlansWrite, h.createService, "shippin.services.catalog_service.create", "Add a service to the catalog."},
		{"GET /v1/operator/catalog/services/{id}", PermPlansRead, h.getService, "shippin.services.catalog_service.get", "Read one catalog service."},
		{"PUT /v1/operator/catalog/services/{id}", PermPlansWrite, h.updateService, "shippin.services.catalog_service.update", "Change a catalog service."},
		{"DELETE /v1/operator/catalog/services/{id}", PermPlansWrite, h.deleteService, "shippin.services.catalog_service.delete", "Remove a service from the catalog."},
		{"GET /v1/operator/catalog/plan-services", PermPlansRead, h.listPlanServices, "shippin.tenancy.plan_service.list", "List which services each plan includes."},
		{"PUT /v1/operator/catalog/plan-services/{plan}/{service}", PermPlansWrite, h.putPlanService, "shippin.tenancy.plan_service.set", "Include a service in a plan, with its lock mode and limits."},
		{"DELETE /v1/operator/catalog/plan-services/{plan}/{service}", PermPlansWrite, h.deletePlanService, "shippin.tenancy.plan_service.remove", "Take a service out of a plan."},
		{"GET /v1/operator/catalog/defaults", PermPlansRead, h.listDefaults, "shippin.tenancy.default.list", "List the configuration defaults by layer."},
		{"PUT /v1/operator/catalog/defaults", PermDefaultsWrite, h.putDefault, "shippin.tenancy.default.set", "Set a configuration default."},
		{"DELETE /v1/operator/catalog/defaults/{layer}/{key}", PermDefaultsWrite, h.deleteDefault, "shippin.tenancy.default.remove", "Remove a configuration default."},
		{"GET /v1/operator/customers", PermCustomersRead, h.listCustomers, "shippin.tenancy.customer.list", "List customers with their plan and coverage."},
		{"POST /v1/operator/customers", PermCustomersWrite, h.createCustomer, "shippin.tenancy.customer.create", "Create a customer on a plan."},
		{"GET /v1/operator/customers/{id}", PermCustomersRead, h.getCustomer, "shippin.tenancy.customer.get", "Read one customer."},
		{"PUT /v1/operator/customers/{id}", PermCustomersWrite, h.updateCustomer, "shippin.tenancy.customer.update", "Change a customer's name, plan, coverage, contacts or notes."},
		{"DELETE /v1/operator/customers/{id}", PermCustomersWrite, h.deleteCustomer, "shippin.tenancy.customer.delete", "Remove a draft customer."},
		{"GET /v1/operator/customers/{id}/services", PermCustomersRead, h.listTenantServices, "shippin.tenancy.customer_service.list", "List a customer's service overrides."},
		{"PUT /v1/operator/customers/{id}/services/{service}", PermServicesWrite, h.putTenantService, "shippin.tenancy.customer_service.set", "Override a service for one customer."},
		{"DELETE /v1/operator/customers/{id}/services/{service}", PermServicesWrite, h.deleteTenantService, "shippin.tenancy.customer_service.remove", "Remove a customer's service override."},
		{"GET /v1/operator/customers/{id}/effective-config", PermEffectiveRead, h.effective, "shippin.tenancy.customer_config.get", "Read a customer's effective configuration after every layer."},
		{"GET /v1/operator/customers/{id}/provision-intents", PermCustomersRead, h.listIntents, "shippin.tenancy.provision_intent.list", "List a customer's provisioning intents."},
		{"POST /v1/operator/customers/{id}/provision-intent", PermIntentCreate, h.provisionIntent, "shippin.tenancy.provision_intent.create", "Record an intent to provision a customer."},
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
