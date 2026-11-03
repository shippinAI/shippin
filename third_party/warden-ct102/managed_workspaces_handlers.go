package main

import (
	"encoding/json"
	"log"
	"net/http"
)

func handleManagedWorkspaces(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/workspaces" {
		http.NotFound(w, r)
		return
	}
	hd := buildHeaderData(r)
	hd.PageTitle = "Workspace Map"
	data := struct {
		HeaderData
		Shell ShellArgs
		Map   ManagedWorkspaceMap
	}{
		HeaderData: hd,
		Shell: ShellArgs{
			HeaderData:   hd,
			PageTitle:    "Workspace Map",
			Breadcrumb:   []BreadcrumbCrumb{{Label: "Workspace Map", Href: "/workspaces", Last: true}},
			BodyClass:    "c-workspace-map",
			BodyTemplate: "workspaces.body",
		},
		Map: getManagedWorkspaceMap(),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "workspaces.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}

func handleAPIManagedWorkspaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"map":     getManagedWorkspaceMap(),
	})
}
