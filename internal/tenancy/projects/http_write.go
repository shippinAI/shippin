package projects

import (
	"errors"
	"log"
	"net/http"

	"shippin.cloud/console/internal/auth"
)

func (h *Handler) create(w http.ResponseWriter, r *http.Request, t, actor string) {
	var in struct {
		Name        string `json:"name"`
		Environment string `json:"environment"`
		WorkspaceID string `json:"workspace_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Environment == "" {
		in.Environment = "dev"
	}
	if !projectName.MatchString(in.Name) || !envName.MatchString(in.Environment) {
		auth.Problem(w, http.StatusBadRequest, "Use lowercase letters, digits and dashes, starting with a letter.")
		return
	}
	id, err := h.S.CreateProjectInWorkspace(r.Context(), t, in.WorkspaceID, in.Name, in.Environment, actor)
	if err != nil {
		writeErr(w, err, "A project with that name already exists.")
		return
	}
	out, err := h.S.Project(r.Context(), t, id)
	reply(w, http.StatusCreated, out, err)
}

func (h *Handler) remove(w http.ResponseWriter, r *http.Request, t, _ string) {
	if err := h.S.DeleteProject(r.Context(), t, r.PathValue("projectId")); err != nil {
		if errors.Is(err, ErrNotFound) {
			err = errInUse
		}
		writeErr(w, err, "")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) addEnv(w http.ResponseWriter, r *http.Request, t, _ string) {
	var in struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !envName.MatchString(in.Name) {
		auth.Problem(w, http.StatusBadRequest, "Use lowercase letters, digits and dashes, starting with a letter.")
		return
	}
	if _, err := h.S.AddEnvironment(r.Context(), t, r.PathValue("projectId"), in.Name); err != nil {
		writeErr(w, err, "An environment with that name already exists.")
		return
	}
	out, err := h.S.Project(r.Context(), t, r.PathValue("projectId"))
	reply(w, http.StatusCreated, out, err)
}

// errInUse: DeleteProject matched no row (unknown project, or one that still has workspaces).
var errInUse = errors.New("in use")

// writeErr maps store errors to problems; anything unexpected is a logged 500, never a 409.
func writeErr(w http.ResponseWriter, err error, taken string) {
	switch {
	case errors.Is(err, ErrWorkspaceInput):
		auth.Problem(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrWorkspaceOccupied):
		auth.Problem(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrNameTaken):
		auth.Problem(w, http.StatusConflict, taken)
	case errors.Is(err, ErrNotFound):
		auth.Problem(w, http.StatusNotFound, "Project not found.")
	case errors.Is(err, errInUse):
		auth.Problem(w, http.StatusConflict, "Project not found, or it still has workspaces.")
	default:
		log.Printf("projects: %v", err)
		auth.Problem(w, http.StatusInternalServerError, "The project store failed. Try again.")
	}
}
