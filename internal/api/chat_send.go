package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/chat"
	"shippin.cloud/console/internal/db"
)

// chatSession loads the caller's session and re-checks they may still use its workspace.
func (a *API) chatSession(w http.ResponseWriter, r *http.Request) (*caller, *chat.Session, *db.Workspace, *tenantView, bool) {
	c, ok := a.chatOn(w, r)
	if !ok {
		return nil, nil, nil, nil, false
	}
	s, err := a.n2.ch.Get(r.Context(), c.id.Sub, r.PathValue("id"))
	if err != nil || s == nil {
		auth.Problem(w, http.StatusNotFound, "Chat not found.")
		return nil, nil, nil, nil, false
	}
	ws, t, ok := a.bridgeWorkspace(w, r, c, s.WorkspaceID)
	return c, s, ws, t, ok
}

func (a *API) chatGet(w http.ResponseWriter, r *http.Request) {
	c, ok := a.chatOn(w, r)
	if !ok {
		return
	}
	s, err := a.n2.ch.Get(r.Context(), c.id.Sub, r.PathValue("id"))
	if err != nil || s == nil {
		auth.Problem(w, http.StatusNotFound, "Chat not found.")
		return
	}
	msgs, err := a.n2.ch.Messages(r.Context(), s.ID)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Messages couldn't be loaded.")
		return
	}
	writeJSON(w, map[string]any{"session": s, "messages": msgs})
}

// chatReady runs the provider's check in the workspace: installed and signed in, or what to do.
func (a *API) chatReady(w http.ResponseWriter, r *http.Request) {
	_, s, ws, t, ok := a.chatSession(w, r)
	if !ok {
		return
	}
	p, _ := a.chatProvider(s.Provider)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	p.Command = p.Check // the check command ignores stdin and exits 0 when signed in
	res := chat.Turn(ctx, a.host, a.cfg.Chat, p, t.ID, ws.ID, s.ID, "", func(string) {})
	var ui []chat.Block
	if res.Err != "" {
		ui = chat.ConnectBlocks(p.Name)
	}
	writeJSON(w, map[string]any{"ready": res.Err == "", "detail": res.Err, "ui": ui, "provider": p.ID, "subscription": p.Subscription,
		"connect_href": "/account/ai", "workspace": ws.Name})
}

// chatSend stores the user's message, streams the CLI's reply as server-sent events
// (delta, done, error) and stores the reply with its validated A2UI blocks.
func (a *API) chatSend(w http.ResponseWriter, r *http.Request) {
	c, s, ws, t, ok := a.chatSession(w, r)
	if !ok {
		return
	}
	var in struct {
		Text string `json:"text"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in) != nil || strings.TrimSpace(in.Text) == "" {
		auth.Problem(w, http.StatusBadRequest, "text is required.")
		return
	}
	p, _ := a.chatProvider(s.Provider)
	history, err := a.n2.ch.Messages(r.Context(), s.ID)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Messages couldn't be loaded.")
		return
	}
	um, err := a.n2.ch.Add(r.Context(), s.ID, "user", in.Text, nil)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The message couldn't be saved.")
		return
	}
	_ = a.store().AddAudit(r.Context(), db.AuditEntry{TenantID: t.ID, TenantName: t.Name, WorkspaceID: ws.ID, WorkspaceName: ws.Name,
		Actor: c.id.Sub, ActorName: c.id.Name, Action: "chat.turn", Detail: fmt.Sprintf("session %s provider %s, %d chars", s.ID, p.ID, len(in.Text)), IP: clientIP(r)})
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	fl, _ := w.(http.Flusher)
	send := func(ev string, v any) {
		b, _ := json.Marshal(v)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev, b)
		if fl != nil {
			fl.Flush()
		}
	}
	send("user", um)
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(a.cfg.Chat.TimeoutSeconds)*time.Second)
	defer cancel()
	if a.chatRRFlowBound(r.Context(), t.ID, ws) {
		history = nil // rrflow/state + the CLI's own session carry the conversation (chat_rrflow.go)
	}
	res := chat.Turn(ctx, a.host, a.cfg.Chat, p, t.ID, ws.ID, s.ID, chat.Prompt(a.cfg.Chat, history, in.Text), func(d string) { send("delta", map[string]string{"text": d}) })
	role, text := "assistant", res.Text
	if res.Err != "" {
		role, text, res.Blocks = "error", res.Err, chat.ConnectBlocks(p.Name)
	}
	m, err := a.n2.ch.Add(context.WithoutCancel(r.Context()), s.ID, role, text, res.Blocks)
	if err != nil {
		send("error", map[string]string{"detail": "The reply couldn't be saved."})
		return
	}
	send("done", m)
}
