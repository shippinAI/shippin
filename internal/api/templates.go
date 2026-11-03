package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
)

// listTemplates: the customer gallery (enabled templates) plus the size presets.
func (a *API) listTemplates(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.caller(w, r); !ok {
		return
	}
	ts, err := a.db.Templates(r.Context(), true)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Templates are unavailable right now.")
		return
	}
	for i := range ts {
		ts[i].Startup = "" // operator detail; the gallery doesn't need it
	}
	writeJSON(w, map[string]any{"items": ts, "sizes": a.cfg.Workspaces.Sizes, "limits": a.templateRules()})
}

// templateRules tells the UI what the server accepts (repo hosts, image prefix, ranges).
func (a *API) templateRules() map[string]any {
	c := a.cfg.Workspaces
	return map[string]any{"repo_hosts": c.RepoHosts, "image_prefix": c.ImagePrefix,
		"preview_min": c.PreviewMin, "preview_max": c.PreviewMax, "template": c.TemplateLimits}
}

// SeedTemplates inserts templates from the configured seed file (by slug; existing ones are kept).
func (a *API) SeedTemplates(ctx context.Context) {
	path := a.cfg.Workspaces.TemplateSeed
	if path == "" {
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		log.Printf("template seed: %v", err)
		return
	}
	ts, errs := parseSeed(raw, a.cfg.Workspaces)
	for _, e := range errs {
		log.Printf("template seed: %v", e)
	}
	for _, t := range ts {
		if err := a.db.SaveTemplate(ctx, t, true); err != nil {
			log.Printf("template seed %s: %v", t.Slug, err)
		}
	}
}

// parseSeed decodes a seed file and keeps only templates that pass checkTemplate (ids filled in).
func parseSeed(raw []byte, c config.Workspaces) ([]db.Template, []error) {
	var in []db.Template
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, []error{err}
	}
	out, errs, seen := []db.Template{}, []error{}, map[string]bool{}
	for _, t := range in {
		if msg := checkTemplate(&t, c); msg != "" {
			errs = append(errs, fmt.Errorf("%q skipped: %s", t.Slug, msg))
			continue
		}
		if seen[t.Slug] {
			errs = append(errs, fmt.Errorf("%q skipped: duplicate slug", t.Slug))
			continue
		}
		seen[t.Slug] = true
		if uuid.Validate(t.ID) != nil {
			id, _ := uuid.NewV7()
			t.ID = id.String()
		}
		out = append(out, t)
	}
	return out, errs
}
