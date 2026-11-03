package auth

import (
	"encoding/json"
	"net/http"
)

// Problem writes an RFC 9457 problem+json body with a toast-friendly message.
func Problem(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "about:blank", "title": http.StatusText(status), "status": status, "message": msg,
	})
}
