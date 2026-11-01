package main

// Runners page + stats JSON surface.
// Spec: docs/specs/runner-fleet-health-and-stats.md §5 (P3).

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type RunnersPageData struct {
	HeaderData
	Runners []CIRunnerHealth
	Stats   CIStats
}

func handleRunners(w http.ResponseWriter, r *http.Request) {
	hd := buildHeaderData(r)
	hd.PageTitle = "Runners"

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	data := RunnersPageData{
		HeaderData: hd,
		Runners:    collectCIRunnerHealth(),
		Stats:      collectCIStats(ctx),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "runners.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleAPICIStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(collectCIStats(ctx))
}
