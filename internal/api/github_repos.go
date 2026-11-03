package api

import (
	"net/http"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/gh"
)

type installationRepos struct {
	ID           int64     `json:"installation_id"`
	AccountLogin string    `json:"account_login"`
	Suspended    bool      `json:"suspended"`
	Repos        []gh.Repo `json:"repos"`
	Error        string    `json:"error,omitempty"`
}

// githubRepos lists the repositories of this tenant's live installations (the import picker).
func (a *API) githubRepos(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.tenantCtx(w, r)
	if !ok || !a.githubFor(w, t) {
		return
	}
	ins, err := a.db.Installations(r.Context(), t.ID)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "GitHub is unavailable right now.")
		return
	}
	out := []installationRepos{}
	for _, in := range ins {
		row := installationRepos{ID: in.ID, AccountLogin: in.AccountLogin, Suspended: in.Suspended, Repos: []gh.Repo{}}
		if !in.Suspended {
			if rs, err := a.gh.Repos(r.Context(), in.ID, a.cfg.GitHub.MaxRepos); err != nil {
				row.Error = "GitHub didn't list this installation's repositories."
			} else {
				row.Repos = rs
			}
		}
		out = append(out, row)
	}
	writeJSON(w, map[string]any{"installations": out, "can_install": c.canDecide(t)})
}
