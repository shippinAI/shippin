package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

func handleAPIInfrastructure(w http.ResponseWriter, r *http.Request) {
	stats := collectStats()
	graph := getInfrastructureGraph(stats)
	if r.URL.Path == "/api/infrastructure" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(graph)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/infrastructure/")
	if rest == r.URL.Path || strings.TrimSpace(rest) == "" {
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(rest, "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	providerID, targetID := parts[0], parts[1]
	for _, p := range graph.Providers {
		if p.ID != providerID {
			continue
		}
		for _, h := range p.Hosts {
			if h.ID == targetID || h.Name == targetID {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(h)
				return
			}
			for _, c := range h.Customers {
				if c.ID == targetID || c.Label == targetID {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(c)
					return
				}
				for _, rsrc := range c.Resources {
					if rsrc.ID == targetID || rsrc.Name == targetID {
						w.Header().Set("Content-Type", "application/json")
						_ = json.NewEncoder(w).Encode(rsrc)
						return
					}
				}
			}
		}
	}
	http.NotFound(w, r)
}
