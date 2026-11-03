package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/projects"
)

// demoWS answers the workspace routes the devspace UI uses, from real rows in the demo database,
// with a real VS Code server per workspace instead of an inner Incus container.
type demoWS struct {
	d   *db.DB
	t   string
	eng *engines
}

type wsView struct {
	db.Workspace
	URL string `json:"url"`
}

func (s *demoWS) view(w db.Workspace) wsView {
	if s.eng.running(w.ID) {
		w.State = "running"
	} else if w.State == "running" {
		w.State = "stopped"
	}
	return wsView{w, fmt.Sprintf("http://127.0.0.1:%d/", enginePort(w.SSHPort))}
}

func (s *demoWS) list(w http.ResponseWriter, r *http.Request) {
	rows, err := s.d.Workspaces(r.Context(), s.t)
	if err != nil {
		auth.Problem(w, http.StatusInternalServerError, "Workspaces are unavailable.")
		return
	}
	out := []wsView{}
	for _, row := range rows {
		out = append(out, s.view(row))
	}
	writeDemo(w, http.StatusOK, map[string]any{"items": out, "can_manage": true, "tenant_status": "active", "plan": map[string]any{"name": "founder"}})
}

func (s *demoWS) create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name    string `json:"name"`
		Project string `json:"project_id"`
		Env     string `json:"environment"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in) != nil || in.Project == "" {
		auth.Problem(w, http.StatusBadRequest, "Invalid request body.")
		return
	}
	pl, err := (&projects.Store{Pool: s.d.Pool}).Place(r.Context(), s.t, in.Project, in.Env)
	if err != nil {
		auth.Problem(w, http.StatusConflict, "That project or environment isn't ready.")
		return
	}
	id, _ := uuid.NewV7()
	ws := db.Workspace{ID: id.String(), TenantID: s.t, Name: in.Name, CreatedBy: "demo", Host: in.Name + ".demo.localhost",
		ProjectID: in.Project, EnvironmentID: pl.EnvID}
	if pl.Boundary != nil {
		ws.BoundaryID = *pl.Boundary
	}
	if err := s.d.CreateWorkspace(r.Context(), &ws, 22000, 22999); err != nil {
		auth.Problem(w, http.StatusConflict, "A workspace with that name already exists.")
		return
	}
	s.boot(ws)
	writeDemo(w, http.StatusAccepted, s.view(ws))
}

func (s *demoWS) boot(ws db.Workspace) {
	state, msg := "running", ""
	if err := s.eng.start(ws.ID, ws.Name, enginePort(ws.SSHPort)); err != nil {
		state, msg = "failed", err.Error()
	}
	_ = s.d.SetWorkspaceState(contextBG(), ws.ID, state, "127.0.0.1", msg)
}

func (s *demoWS) action(w http.ResponseWriter, r *http.Request) {
	rows, _ := s.d.Workspaces(r.Context(), s.t)
	for _, ws := range rows {
		if ws.ID != r.PathValue("workspaceId") {
			continue
		}
		switch r.PathValue("action") {
		case "start":
			s.boot(ws)
		case "stop":
			s.eng.stop(ws.ID)
			_ = s.d.SetWorkspaceState(r.Context(), ws.ID, "stopped", "", "")
		default:
			auth.Problem(w, http.StatusBadRequest, "The demo supports start and stop.")
			return
		}
		writeDemo(w, http.StatusAccepted, map[string]string{"status": "accepted"})
		return
	}
	auth.Problem(w, http.StatusNotFound, "Workspace not found.")
}

func writeDemo(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
