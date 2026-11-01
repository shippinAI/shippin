package catalog

import (
	"encoding/json"
	"net/http"

	"shippin.cloud/console/internal/auth"
)

func (h *Handler) listServices(w http.ResponseWriter, r *http.Request, _ string) {
	raw, err := h.S.Services(r.Context())
	items(w, raw, err, "Services")
}

func (h *Handler) getService(w http.ResponseWriter, r *http.Request, _ string) {
	raw, err := h.S.Service(r.Context(), r.PathValue("id"))
	one(w, raw, err, "Service")
}

func (h *Handler) readService(w http.ResponseWriter, r *http.Request) (Service, bool) {
	v := Service{Active: true, Kind: "app"}
	if !decodeBody(w, r, &v) {
		return v, false
	}
	if id := r.PathValue("id"); id != "" {
		v.ID = id
	}
	for _, f := range []*json.RawMessage{&v.ConfigSchema, &v.DefaultConfig} {
		if len(*f) == 0 {
			*f = json.RawMessage(`{}`)
		}
	}
	var a, b map[string]any
	if !slug.MatchString(v.ID) || v.Label == "" || json.Unmarshal(v.ConfigSchema, &a) != nil || json.Unmarshal(v.DefaultConfig, &b) != nil {
		auth.Problem(w, http.StatusBadRequest, "A service needs a lowercase id, a label, and JSON objects for config schema and defaults.")
		return v, false
	}
	return v, true
}

func (h *Handler) createService(w http.ResponseWriter, r *http.Request, _ string) {
	if v, ok := h.readService(w, r); ok {
		if err := h.S.CreateService(r.Context(), v); err != nil {
			fail(w, err, "Service")
			return
		}
		writeJSON(w, http.StatusCreated, v)
	}
}

func (h *Handler) updateService(w http.ResponseWriter, r *http.Request, _ string) {
	if v, ok := h.readService(w, r); ok {
		if err := h.S.UpdateService(r.Context(), v); err != nil {
			fail(w, err, "Service")
			return
		}
		writeJSON(w, http.StatusOK, v)
	}
}

func (h *Handler) deleteService(w http.ResponseWriter, r *http.Request, _ string) {
	if err := h.S.DeleteService(r.Context(), r.PathValue("id")); err != nil {
		fail(w, err, "Service")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listDefaults(w http.ResponseWriter, r *http.Request, _ string) {
	q := r.URL.Query()
	raw, err := h.S.Defaults(r.Context(), q.Get("layer"), q.Get("ref"))
	items(w, raw, err, "Defaults")
}

func (h *Handler) putDefault(w http.ResponseWriter, r *http.Request, _ string) {
	var it Item
	if !decodeBody(w, r, &it) {
		return
	}
	it.Lock = lockOr(it.Lock)
	if layerRank(it.Layer) >= len(LayerOrder) || it.Key == "" || !validLock(it.Lock) || !json.Valid(it.Value) {
		auth.Problem(w, http.StatusBadRequest, "A default needs a layer (system, plan, tenant, workspace), a key, a JSON value and a lock mode.")
		return
	}
	if (it.Layer == "system") != (it.Ref == "") {
		auth.Problem(w, http.StatusBadRequest, "System defaults have no ref; every other layer needs one (plan, tenant or workspace id).")
		return
	}
	if err := h.S.PutDefault(r.Context(), it); err != nil {
		fail(w, err, "Default")
		return
	}
	writeJSON(w, http.StatusOK, it)
}

func (h *Handler) deleteDefault(w http.ResponseWriter, r *http.Request, _ string) {
	err := h.S.DeleteDefault(r.Context(), r.PathValue("layer"), r.URL.Query().Get("ref"), r.PathValue("key"))
	if err != nil {
		fail(w, err, "Default")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
