package api

import (
	"context"
	"log"
	"time"

	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/managed"
)

// Chat state per 02c-rrflow.md: the project's rrflow/state instance (on the tenant boundary VM)
// holds each chat's session, the CLI's own resumable session id (memory chat/<id>/…) and one
// decision per turn. The in-workspace wrapper (hostd chat session_commands) reads and writes it;
// the console only decides whether to stop resending history.

// chatRRFlowBound reports whether the workspace has a bound rrflow/state instance.
func (a *API) chatRRFlowBound(ctx context.Context, tenant string, ws *db.Workspace) bool {
	if a.svcs == nil || ws.ProjectID == "" {
		return false
	}
	insts, err := a.svcs.ByProject(ctx, tenant, ws.ProjectID)
	if err != nil {
		return false
	}
	for _, in := range insts {
		if in.Kind != managed.KindRRFlow || in.State != "ready" {
			continue
		}
		for _, b := range in.Bindings {
			if b.WorkspaceID == ws.ID && b.State == "bound" {
				return true
			}
		}
	}
	return false
}

// chatAttachRRFlow ensures rrflow/state is bound to the chat's workspace, in the background:
// a new chat works at once (history fallback) and switches to rrflow when the binding is ready.
func (a *API) chatAttachRRFlow(tenant string, ws *db.Workspace, by string) {
	if a.svcs == nil || ws.ProjectID == "" || a.chatRRFlowBound(context.Background(), tenant, ws) {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := a.attachRRFlow(ctx, tenant, ws.ProjectID, ws.ID, by); err != nil {
			log.Printf("chat: rrflow attach %s: %v", ws.ID, err)
		}
	}()
}
