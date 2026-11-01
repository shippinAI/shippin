package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

func handleAPIPlatformSystems(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"systems": listPlatformSystems()})
	case http.MethodPost:
		handleAPIPlatformSystemsPost(w, r)
	case http.MethodDelete:
		handleAPIPlatformSystemsDelete(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleAPIPlatformSystemsPost(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var ps PlatformSystem
	if err := json.NewDecoder(r.Body).Decode(&ps); err != nil {
		jsonError(w, "Invalid JSON: "+err.Error())
		return
	}
	ps.Name = strings.TrimSpace(ps.Name)
	if ps.ID != "" {
		ps.ID = slugifyID(ps.ID)
	} else {
		ps.ID = slugifyID(ps.Name)
	}
	ps.Category = strings.TrimSpace(ps.Category)
	ps.Scope = strings.TrimSpace(ps.Scope)
	ps.PublicURL = strings.TrimSpace(ps.PublicURL)
	ps.CheckHost = strings.TrimSpace(ps.CheckHost)
	ps.Notes = strings.TrimSpace(ps.Notes)
	if err := appendPlatformSystem(ps); err != nil {
		jsonError(w, err.Error())
		return
	}
	auditFromRequest(r, "platform_system_add", "Added platform system "+ps.ID, "dashboard")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "id": ps.ID})
}

func handleAPIPlatformSystemsDelete(w http.ResponseWriter, r *http.Request) {
	id := slugifyID(strings.TrimSpace(r.URL.Query().Get("id")))
	if err := removePlatformSystem(id); err != nil {
		jsonError(w, err.Error())
		return
	}
	auditFromRequest(r, "platform_system_remove", "Removed platform system "+id, "dashboard")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}
