package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/agents"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

// agentDeps: the store and the IdP call that creates the service user (test seams; nil = real).
type agentDeps struct {
	store *agents.Store
	idp   func(ctx context.Context, org, userName, name, desc string) (string, error)
}

func (a *API) agentStore() *agents.Store {
	if a.agents.store != nil {
		return a.agents.store
	}
	return &agents.Store{Pool: a.db.Pool}
}

func (a *API) agentIdP(ctx context.Context, org, userName, name, desc string) (string, error) {
	if a.agents.idp != nil {
		return a.agents.idp(ctx, org, userName, name, desc)
	}
	return a.flow.CreateMachineUser(ctx, a.cfg.IDPServiceToken, org, userName, name, desc)
}

// agentRoutes (spec identity.yaml#/agents): members read; an org owner/admin creates, and the new
// agent is delegated by them. The IdP holds the identity; no credential is created or returned.
func (a *API) agentRoutes(mux Router) {
	mux.HandleFunc("GET /v1/tenants/{tenantId}/identity/agents", a.flow.Require(a.listAgents))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/identity/agents", a.flow.Require(a.createAgent))
}

func (a *API) listAgents(w http.ResponseWriter, r *http.Request) {
	_, t, ok := a.tenantCtx(w, r)
	if !ok {
		return
	}
	if !t.member() {
		auth.Problem(w, http.StatusForbidden, "Only members of this tenant can see its agents.")
		return
	}
	items, err := a.agentStore().List(r.Context(), t.ID)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Agents are unavailable right now.")
		return
	}
	writeJSON(w, map[string]any{"items": items, "page": map[string]any{"next_page_token": nil, "total_estimate": len(items)},
		"enabled": a.cfg.Agents.Enabled})
}

type agentCreate struct {
	Name        string `json:"name"`
	WorkspaceID string `json:"workspace_id"`
}

func (a *API) createAgent(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.tenantCtx(w, r)
	if !ok {
		return
	}
	switch {
	case !a.cfg.Agents.Enabled:
		auth.Problem(w, http.StatusNotFound, "Agent identities aren't enabled.")
	case !c.canDecide(t):
		auth.Problem(w, http.StatusForbidden, "Creating agents needs the org owner or admin role.")
	default:
		a.idempotent(w, r, t.ID+":"+c.id.Sub, func(w http.ResponseWriter, r *http.Request) { a.doCreateAgent(w, r, c, t) })
	}
}

func (a *API) doCreateAgent(w http.ResponseWriter, r *http.Request, c *caller, t *tenantView) {
	var in agentCreate
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil {
		auth.Problem(w, http.StatusBadRequest, "Send a JSON body with a name.")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if n := utf8.RuneCountInString(in.Name); n < 1 || n > 120 || strings.ContainsAny(in.Name, "\r\n\t") {
		auth.Problem(w, http.StatusBadRequest, "The name must be 1 to 120 characters on one line.")
		return
	}
	if msg := a.checkAgentPlace(r, t, in); msg != "" {
		auth.Problem(w, http.StatusConflict, msg)
		return
	}
	id, _ := uuid.NewV7()
	ag := &agents.Agent{ID: id.String(), TenantID: t.ID, Name: in.Name, WorkspaceID: in.WorkspaceID, DelegatingUserID: c.id.Sub}
	e := db.AuditEntry{TenantID: t.ID, WorkspaceID: in.WorkspaceID, Actor: c.id.Sub, ActorName: displayName(c),
		Action: "agent.create", Detail: ag.ID + " " + in.Name, IP: clientIP(r)}
	if !a.auditFirst(w, r, e) {
		return
	}
	uid, err := a.agentIdP(r.Context(), t.IdpOrgID, a.cfg.Agents.UserPrefix+ag.ID, in.Name, "Agent delegated by "+c.id.Sub)
	if err != nil {
		log.Printf("agent %s: idp: %v", ag.ID, err)
		auth.Problem(w, http.StatusBadGateway, "The identity provider didn't create the agent. Try again.")
		return
	}
	ag.IdPUserID = uid
	if err := a.agentStore().Create(r.Context(), ag); errors.Is(err, agents.ErrNameTaken) {
		auth.Problem(w, http.StatusConflict, "An agent with that name already exists.")
		return
	} else if err != nil {
		log.Printf("agent %s: store (idp user %s left for cleanup): %v", ag.ID, uid, err)
		auth.Problem(w, http.StatusServiceUnavailable, "The agent couldn't be saved. Try again.")
		return
	}
	writeStatus(w, http.StatusCreated, ag)
}

// checkAgentPlace: cap, unique name and (optional) a live workspace of this tenant. "" = ok.
func (a *API) checkAgentPlace(r *http.Request, t *tenantView, in agentCreate) string {
	s := a.agentStore()
	if n, err := s.Count(r.Context(), t.ID); err != nil || n >= a.cfg.Agents.MaxPerTenant {
		return "This tenant has reached its agent limit."
	}
	if free, err := s.NameFree(r.Context(), t.ID, in.Name); err != nil || !free {
		return "An agent with that name already exists."
	}
	if in.WorkspaceID == "" {
		return ""
	}
	ws, err := a.db.Workspaces(r.Context(), t.ID)
	if err == nil {
		for _, x := range ws {
			if x.ID == in.WorkspaceID {
				return ""
			}
		}
	}
	return "That workspace isn't in this tenant."
}
