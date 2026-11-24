package hub

import (
	"encoding/json"
	"errors"
	"net/http"

	"shippin.cloud/console/internal/auth"
)

func (h *Handler) out(c *call, v any, err error) { h.outStatus(c, http.StatusOK, v, err) }

func (h *Handler) outStatus(c *call, code int, v any, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		auth.Problem(c.w, http.StatusNotFound, "No such task.")
	case errors.Is(err, ErrConflict):
		auth.Problem(c.w, http.StatusConflict, "The task's state or lease does not allow this.")
	case err != nil:
		auth.Problem(c.w, http.StatusServiceUnavailable, "The hub store is unavailable.")
	default:
		reply(c.w, code, v)
	}
}

func reply(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
