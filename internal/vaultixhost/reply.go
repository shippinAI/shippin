package vaultixhost

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"shippin.cloud/console/internal/vaultix"
)

func reply(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func problem(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": code, "message": msg})
}

// isMissing spots Infisical's "secret not found" answers that are not plain 404s.
func isMissing(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "not found") || strings.Contains(s, "does not exist")
}

func upstream(w http.ResponseWriter, err error) {
	if errors.Is(err, vaultix.ErrNotFound) {
		problem(w, http.StatusNotFound, "Not found.")
		return
	}
	log.Printf("vaultix: %v", err) // messages carry paths and status, never values
	problem(w, http.StatusBadGateway, "Vaultix rejected the request.")
}
