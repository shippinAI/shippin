package main

// HTTP surface for the CI runner fleet health layer.
// Spec: docs/specs/runner-fleet-health-and-stats.md §2 (P1).

import (
	"encoding/json"
	"net/http"
	"time"
)

func handleAPIRunners(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	health := collectCIRunnerHealth()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"runners":    health,
		"checked_at": time.Now().UTC(),
	})
}
