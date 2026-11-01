package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/chat"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
)

// Console chat: server-backed sessions (the overlay and /chat read the same rows), answered by
// the user's own AI CLI in their workspace via hostd. No route accepts or returns a credential.
func (a *API) chatRoutes(mux Router) {
	mux.HandleFunc("GET /v1/chat/config", a.flow.Require(a.chatConfig))
	mux.HandleFunc("GET /v1/chat/tips", a.flow.Require(a.chatTips))
	mux.HandleFunc("GET /v1/chat/sessions", a.flow.Require(a.chatList))
	mux.HandleFunc("POST /v1/chat/sessions", a.flow.Require(a.chatCreate))
	mux.HandleFunc("GET /v1/chat/sessions/{id}", a.flow.Require(a.chatGet))
	mux.HandleFunc("GET /v1/chat/sessions/{id}/ready", a.flow.Require(a.chatReady))
	mux.HandleFunc("POST /v1/chat/sessions/{id}/messages", a.flow.Require(a.chatSend))
}

func (a *API) chatOn(w http.ResponseWriter, r *http.Request) (*caller, bool) {
	if a.n2.ch == nil {
		auth.Problem(w, http.StatusNotFound, "Chat isn't enabled.")
		return nil, false
	}
	return a.caller(w, r)
}

func (a *API) chatProvider(id string) (config.ChatProvider, bool) {
	for _, p := range a.cfg.Chat.Providers {
		if p.ID == id {
			return p, true
		}
	}
	return config.ChatProvider{}, false
}

func (a *API) chatConfig(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.chatOn(w, r); !ok {
		return
	}
	writeJSON(w, map[string]any{"providers": a.cfg.Chat.Providers, "catalog": chat.Catalog})
}

// chatTips: passive knowledge panels for the page the user is on (longest route prefix first).
func (a *API) chatTips(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.chatOn(w, r); !ok {
		return
	}
	route := r.URL.Query().Get("route")
	out := []chat.Block{}
	for _, t := range a.cfg.Chat.Tips {
		if strings.HasPrefix(route, t.Route) || (strings.Contains(t.Route, "*") && matchRoute(t.Route, route)) {
			if b, ok := chat.Validate(chat.Block{Type: "tip", Title: t.Title, Body: t.Body, Href: t.Href}, 0); ok {
				out = append(out, b)
			}
		}
	}
	writeJSON(w, map[string]any{"items": out})
}

// matchRoute: "*" matches one path segment.
func matchRoute(pattern, route string) bool {
	p, s := strings.Split(pattern, "/"), strings.Split(route, "/")
	if len(s) < len(p) {
		return false
	}
	for i := range p {
		if p[i] != "*" && p[i] != s[i] {
			return false
		}
	}
	return true
}

func (a *API) chatList(w http.ResponseWriter, r *http.Request) {
	c, ok := a.chatOn(w, r)
	if !ok {
		return
	}
	list, err := a.n2.ch.List(r.Context(), c.id.Sub)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Chats couldn't be loaded.")
		return
	}
	writeJSON(w, map[string]any{"items": list})
}

func (a *API) chatCreate(w http.ResponseWriter, r *http.Request) {
	c, ok := a.chatOn(w, r)
	if !ok {
		return
	}
	var in struct {
		WorkspaceID string  `json:"workspace_id"`
		ProjectID   *string `json:"project_id"`
		Provider    string  `json:"provider"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil {
		auth.Problem(w, http.StatusBadRequest, "workspace_id and provider are required.")
		return
	}
	if _, ok := a.chatProvider(in.Provider); !ok {
		auth.Problem(w, http.StatusBadRequest, "Unknown provider.")
		return
	}
	ws, t, ok := a.bridgeWorkspace(w, r, c, in.WorkspaceID)
	if !ok {
		return
	}
	pj := in.ProjectID
	if pj == nil && ws.ProjectID != "" {
		pj = &ws.ProjectID
	}
	s, err := a.n2.ch.Create(r.Context(), chat.Session{TenantID: t.ID, ProjectID: pj, WorkspaceID: ws.ID, Owner: c.id.Sub, Provider: in.Provider})
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The chat couldn't be created.")
		return
	}
	_ = a.store().AddAudit(r.Context(), db.AuditEntry{TenantID: t.ID, TenantName: t.Name, WorkspaceID: ws.ID, WorkspaceName: ws.Name,
		Actor: c.id.Sub, ActorName: c.id.Name, Action: "chat.session_created", Detail: s.ID + " " + in.Provider, IP: clientIP(r)})
	a.chatAttachRRFlow(t.ID, ws, c.id.Sub)
	writeJSON(w, s)
}
