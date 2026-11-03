package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"

	"shippin.cloud/console/internal/agentruns"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/jobs"
	"shippin.cloud/console/internal/projects"
)

type promptIn struct {
	Prompt   string `json:"prompt"`
	Stack    string `json:"stack"`
	Provider string `json:"provider"`
	Name     string `json:"name"`
}

// promptProject (M4, implementation-plan-3): POST /v1/tenants/{t}/projects/from-prompt creates a
// project from a prompt and queues project.from_prompt, which brings up its devspace and runs the
// caller's OWN signed-in CLI agent there. Shippin never sees a token; the prompt only passes through.
func (a *API) promptProject(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.tenantCtx(w, r)
	if !ok {
		return
	}
	if !a.cfg.Prompt.Allows(t.ID) || !a.cfg.AI.Allows(t.ID) {
		auth.Problem(w, http.StatusForbidden, "Building from a prompt isn't available in this tenant yet.")
		return
	}
	if !c.canDecide(t) {
		auth.Problem(w, http.StatusForbidden, "Creating projects needs the org owner or admin role.")
		return
	}
	a.idempotent(w, r, t.ID+":"+c.id.Sub, func(w http.ResponseWriter, r *http.Request) { a.createFromPrompt(w, r, c, t) })
}

func (a *API) createFromPrompt(w http.ResponseWriter, r *http.Request, c *caller, t *tenantView) {
	var in promptIn
	max := a.cfg.Prompt.MaxBytes
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, int64(max)+4096)).Decode(&in) != nil {
		auth.Problem(w, http.StatusBadRequest, "The request couldn't be read.")
		return
	}
	if strings.TrimSpace(in.Prompt) == "" || len(in.Prompt) > max || !utf8.ValidString(in.Prompt) {
		auth.Problem(w, http.StatusBadRequest, "Describe what to build (up to 8 KB of text).")
		return
	}
	p, known := a.cfg.AI.Provider(in.Provider)
	if !known || !a.cfg.Prompt.IsAgent(in.Provider) {
		auth.Problem(w, http.StatusBadRequest, "Pick which of your AI subscriptions should build it.")
		return
	}
	slug := in.Stack
	if slug == "" {
		slug = projects.GuessStack(in.Prompt, a.cfg.Prompt.Stacks, a.cfg.Prompt.DefaultTemplate)
	}
	if spec, msg := a.workspaceSpec(r.Context(), slug, "", ""); msg != "" || spec.Template == "" {
		auth.Problem(w, http.StatusBadRequest, "That stack isn't available.")
		return
	}
	st := &projects.Store{Pool: a.db.Pool}
	name := in.Name
	if name != "" && !projectNameRe.MatchString(name) {
		auth.Problem(w, http.StatusBadRequest, "Name must be 2-40 lowercase letters, digits or dashes.")
		return
	}
	if name == "" {
		var err error
		if name, err = projects.UniqueName(r.Context(), projects.SlugFromPrompt(in.Prompt),
			func(ctx2 context.Context, n string) (bool, error) { return st.NameTaken(ctx2, t.ID, n) }); err != nil {
			auth.Problem(w, http.StatusConflict, "Pick a project name.")
			return
		}
	}
	acct, err := a.db.UpsertAccount(r.Context(), c.id.Sub, c.id.Email)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The project could not be saved.")
		return
	}
	pid, err := st.CreateProject(r.Context(), t.ID, name, "dev", c.id.Sub)
	if errors.Is(err, projects.ErrNameTaken) {
		auth.Problem(w, http.StatusConflict, "A project with that name already exists.")
		return
	}
	if err == nil {
		err = a.db.SetSource(r.Context(), db.ProjectSource{ProjectID: pid, TenantID: t.ID, Kind: "prompt"})
	}
	var run *agentruns.Run
	if err == nil {
		run, err = a.runs.Create(r.Context(), agentruns.Run{TenantID: t.ID, ProjectID: pid, AccountID: acct, CreatedBy: c.id.Sub,
			Provider: p.ID, Template: slug}, in.Prompt)
	}
	if err != nil {
		log.Printf("prompt project: %v", err)
		auth.Problem(w, http.StatusServiceUnavailable, "The project could not be saved.")
		return
	}
	writeStatus(w, http.StatusAccepted, map[string]any{"project_id": pid, "name": name, "template": slug,
		"run": run, "job": a.queuePrompt(r, c, t.ID, pid, run.ID)})
}

// queuePrompt enqueues project.from_prompt (or, jobs disabled, runs it in the background).
func (a *API) queuePrompt(r *http.Request, c *caller, tenant, project, run string) *jobOut {
	payload, _ := json.Marshal(map[string]string{"run_id": run})
	job := jobs.Job{TenantID: tenant, Kind: jobs.KindProjectPrompt, TargetType: "project", TargetID: project,
		MaxAttempts: a.cfg.Jobs.MaxAttempts, CreatedBy: c.id.Sub, Payload: payload}
	if a.runner == nil {
		go a.promptDetached(job)
		return nil
	}
	got, _, err := a.jobs.Enqueue(r.Context(), job)
	if err != nil {
		log.Printf("prompt %s: enqueue: %v", project, err)
		return nil
	}
	a.runner.Wake()
	return jobView(got, c.op != "")
}
