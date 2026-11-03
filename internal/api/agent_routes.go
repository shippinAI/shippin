package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"shippin.cloud/console/internal/agentruns"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/hostclient"
)

// promptRoutes: M4 prompt → project and the build agent's progress (Shippin's own view of it).
func (a *API) promptRoutes(mux Router) {
	mux.HandleFunc("GET /v1/tenants/{tenantId}/prompt/options", a.flow.Require(a.promptOptions))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/projects/from-prompt", a.flow.Require(a.promptProject))
	mux.HandleFunc("GET /v1/tenants/{tenantId}/projects/{projectId}/agent", a.flow.Require(a.projectAgent))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/projects/{projectId}/agent/start", a.flow.Require(a.startProjectAgent))
}

type promptOptionsOut struct {
	Enabled         bool                `json:"enabled"`
	MaxBytes        int                 `json:"max_bytes"`
	Stacks          []config.Stack      `json:"stacks"`
	DefaultTemplate string              `json:"default_template"`
	Agents          []config.AIProvider `json:"agents"`
	TermsNote       string              `json:"terms_note"`
}

// promptOptions tells the New project form what it can offer (all from config).
func (a *API) promptOptions(w http.ResponseWriter, r *http.Request) {
	_, t, ok := a.tenantCtx(w, r)
	if !ok {
		return
	}
	p := a.cfg.Prompt
	out := promptOptionsOut{Enabled: p.Allows(t.ID) && a.cfg.AI.Allows(t.ID), MaxBytes: p.MaxBytes, Stacks: p.Stacks,
		DefaultTemplate: p.DefaultTemplate, Agents: []config.AIProvider{}, TermsNote: a.cfg.AI.TermsNote}
	if out.Stacks == nil {
		out.Stacks = []config.Stack{}
	}
	for _, id := range p.Agents {
		if pr, ok := a.cfg.AI.Provider(id); ok {
			out.Agents = append(out.Agents, pr)
		}
	}
	writeJSON(w, out)
}

type agentOut struct {
	Run       *agentruns.Run          `json:"run"`
	Live      *hostclient.AgentStatus `json:"live,omitempty"`
	TermURL   string                  `json:"term_url,omitempty"`
	Label     string                  `json:"provider_label"`
	Connected bool                    `json:"connected"`
	Mine      bool                    `json:"mine"`
}

// projectAgent: the project's latest build run plus, when its devspace runs, the agent's live
// state (session up, last activity, files changed). run is null when the project has no build.
func (a *API) projectAgent(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.tenantCtx(w, r)
	if !ok {
		return
	}
	if !t.member() {
		auth.Problem(w, http.StatusForbidden, "Only members of this tenant can see its builds.")
		return
	}
	run, err := a.runs.Latest(r.Context(), t.ID, r.PathValue("projectId"))
	if err == agentruns.ErrNotFound {
		writeJSON(w, agentOut{})
		return
	}
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Builds are unavailable right now.")
		return
	}
	out := agentOut{Run: run, Label: run.Provider, Connected: a.agentConnected(r.Context(), run), Mine: run.CreatedBy == c.id.Sub}
	if p, ok := a.cfg.AI.Provider(run.Provider); ok {
		out.Label = p.Label
	}
	if run.WorkspaceID != "" {
		if ws, err := a.db.WorkspaceByID(r.Context(), run.WorkspaceID); err == nil && ws != nil && ws.State == "running" {
			out.TermURL = "https://" + ws.Host + a.cfg.Prompt.TermPath
			ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
			defer cancel()
			if st, err := a.host.AgentStatus(ctx, t.ID, ws.ID); err == nil {
				out.Live = st
				if st.Exited && run.State == agentruns.Running {
					_ = a.runs.SetState(r.Context(), run.ID, agentruns.Exited, "")
					run.State = agentruns.Exited
				}
			}
		}
	}
	writeJSON(w, out)
}

// startProjectAgent starts (or, with resume, continues) the build. Only the run's owner may: the
// agent runs on THEIR subscription, so nobody else can spend it.
func (a *API) startProjectAgent(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.tenantCtx(w, r)
	if !ok {
		return
	}
	run, err := a.runs.Latest(r.Context(), t.ID, r.PathValue("projectId"))
	if err != nil {
		auth.Problem(w, http.StatusNotFound, "This project has no build.")
		return
	}
	if run.CreatedBy != c.id.Sub || !t.member() {
		auth.Problem(w, http.StatusForbidden, "Only the person who started this build can run it, on their own subscription.")
		return
	}
	var in agentStartIn
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&in)
	if !a.agentConnected(r.Context(), run) {
		auth.Problem(w, http.StatusConflict, "Connect your subscription first, then start the build.")
		return
	}
	j := a.queueAgentStart(r.Context(), run, c.id.Sub, in.Resume)
	writeStatus(w, http.StatusAccepted, map[string]any{"run": run, "job": jobView(j, c.op != "")})
}
