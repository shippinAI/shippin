package api

import (
	"encoding/json"
	"net/http"
)

func writeJSON(w http.ResponseWriter, v any) { writeStatus(w, http.StatusOK, v) }

// writeStatus writes v as JSON. no-store: responses may carry a one-time secret (enrollment token).
func writeStatus(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
