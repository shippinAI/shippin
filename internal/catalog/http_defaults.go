package catalog

import (
	"encoding/json"
	"net/http"

	"shippin.cloud/console/internal/auth"
)

// Layered defaults stay legacy routes: a default is named by layer, ref and key together and PUT
// upserts it, which no single-ID action fits yet. Catalog services are actions in
// internal/services/catalog_service.

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
