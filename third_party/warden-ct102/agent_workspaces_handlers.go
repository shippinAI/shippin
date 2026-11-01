package main

import (
	"log"
	"net/http"
)

func handleAgentWorkspaces(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/agent-workspaces" {
		http.NotFound(w, r)
		return
	}

	data := buildAgentWorkspacesPageData(r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "agent_workspaces.html", data); err != nil {
		log.Printf("template error: %v", err)
	}
}
