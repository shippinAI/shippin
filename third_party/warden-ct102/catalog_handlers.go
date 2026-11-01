package main

// Catalog HTTP handlers.
//
// Endpoints:
//   GET  /catalog               — page (grid of templates)
//   GET  /api/catalog           — JSON: full inventory
//   GET  /api/catalog/{name}    — JSON: single template detail
//   GET  /api/catalog?refresh=1 — bust cache, reload, return JSON

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
)

// handleCatalog renders the Catalog page.
func handleCatalog(w http.ResponseWriter, r *http.Request) {
	inv := getCatalog()

	data := CatalogPageData{
		HeaderData: buildHeaderData(r),
		Inventory:  inv,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "catalog.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

// handleAPICatalog serves the catalog as JSON.
//   - GET /api/catalog            → full inventory
//   - GET /api/catalog?refresh=1  → bust cache, reload, return inventory
//   - GET /api/catalog/<name>     → single template
func handleAPICatalog(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("refresh") == "1" {
		invalidateCatalogCache()
	}

	// /api/catalog/<name> — single template detail
	if rest := strings.TrimPrefix(r.URL.Path, "/api/catalog/"); rest != r.URL.Path && rest != "" {
		inv := getCatalog()
		for _, t := range inv.Templates {
			if t.Name == rest {
				w.Header().Set("Content-Type", "application/json")
				if err := json.NewEncoder(w).Encode(t); err != nil {
					log.Printf("Catalog API encode error for template %q: %v", rest, err)
				}
				return
			}
		}
		log.Printf("Catalog API: template %q not found", rest)
		http.Error(w, "template not found", http.StatusNotFound)
		return
	}

	inv := getCatalog()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(inv); err != nil {
		log.Printf("Catalog API encode error: %v", err)
	}
}
