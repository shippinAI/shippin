package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

func (a *API) opTemplates(w http.ResponseWriter, r *http.Request, _ *caller) {
	ts, err := a.db.Templates(r.Context(), false)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Templates are unavailable right now.")
		return
	}
	writeJSON(w, map[string]any{"items": ts, "sizes": a.cfg.Workspaces.Sizes, "limits": a.templateRules()})
}

// opSaveTemplate creates (POST) or updates (PUT /{id}) a template after validation.
func (a *API) opSaveTemplate(w http.ResponseWriter, r *http.Request, c *caller) {
	var t db.Template
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&t) != nil {
		auth.Problem(w, http.StatusBadRequest, "Invalid request body.")
		return
	}
	if id := r.PathValue("templateId"); id != "" {
		if old, err := a.db.Template(r.Context(), id); err != nil || old == nil || old.ID != id {
			auth.Problem(w, http.StatusNotFound, "Template not found.")
			return
		}
		t.ID = id
	} else {
		id, _ := uuid.NewV7()
		t.ID = id.String()
	}
	if msg := checkTemplate(&t, a.cfg.Workspaces); msg != "" {
		auth.Problem(w, http.StatusBadRequest, msg)
		return
	}
	if !a.auditOperator(w, r, c, "", "template.save", t.ID+" "+t.Slug) {
		return
	}
	if err := a.db.SaveTemplate(r.Context(), t, false); err != nil {
		if strings.Contains(err.Error(), "duplicate") {
			auth.Problem(w, http.StatusConflict, "A template with that slug already exists.")
			return
		}
		auth.Problem(w, http.StatusServiceUnavailable, "Template could not be saved.")
		return
	}
	writeJSON(w, t)
}

func (a *API) opDeleteTemplate(w http.ResponseWriter, r *http.Request, c *caller) {
	old, err := a.db.Template(r.Context(), r.PathValue("templateId"))
	if err != nil || old == nil || old.ID != r.PathValue("templateId") {
		auth.Problem(w, http.StatusNotFound, "Template not found.")
		return
	}
	if !a.auditOperator(w, r, c, "", "template.delete", old.ID+" "+old.Slug) {
		return
	}
	if err := a.db.DeleteTemplate(r.Context(), r.PathValue("templateId")); err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Template could not be deleted.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
