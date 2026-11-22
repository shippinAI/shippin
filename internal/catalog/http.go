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

// Route is mounted by the caller (the console gates it by Perm; the demo binary does not). These routes
// predate MCPL actions and move to internal/tenancy/<entity>/<verb>.go one entity at a time.
type Route struct {
	Pattern string
	Perm    string
	H       func(w http.ResponseWriter, r *http.Request, actor string)
}

// Granular permissions (internal/api/perm_catalog.go): deny by default, Owner preset or a
// direct grant only. Route gates are enforced by the console; field gates here.
const (
	PermPlansRead     = "catalog.plans.read"
	PermPricesRead    = "catalog.prices.read"
	PermPlansWrite    = "catalog.plans.write"
	PermDefaultsWrite = "config.defaults.write"
)

func (h *Handler) Routes() []Route {
	return []Route{
		{"GET /v1/operator/catalog/defaults", PermPlansRead, h.listDefaults},
		{"PUT /v1/operator/catalog/defaults", PermDefaultsWrite, h.putDefault},
		{"DELETE /v1/operator/catalog/defaults/{layer}/{key}", PermDefaultsWrite, h.deleteDefault},
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
