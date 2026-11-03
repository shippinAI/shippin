package api

import (
	"context"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"shippin.cloud/console/internal/apps"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

func (a *API) appsRoutes(mux Router) {
	mux.HandleFunc("GET /v1/tenants/{tenantId}/apps", a.flow.Require(a.tenantApps))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/apps/{app}/copy", a.flow.Require(a.tenantAppCopy))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/desktop/prepare", a.flow.Require(a.tenantDesktopPrepare))
	mux.HandleFunc("GET /v1/tenants/{tenantId}/desktop", a.flow.Require(a.desktopSocket))
}

// appsCaller accepts a console session or a shippin-bridge bearer token, so the CLI and the
// buttons hit the same routes.
func (a *API) appsCaller(w http.ResponseWriter, r *http.Request) (*caller, bool) {
	if a.n2.br != nil && a.n2.br.cfg.TokenPrefix != "" {
		tok, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if strings.HasPrefix(tok, a.n2.br.cfg.TokenPrefix) {
			c, _, ok := a.bridgeCaller(w, r)
			return c, ok
		}
	}
	return a.caller(w, r)
}

func (a *API) readApps(r *http.Request, tenant string) (*apps.Status, string) {
	st, err := a.host.AppsStatus(r.Context(), tenant)
	if err != nil {
		return nil, "Status: unknown. The tenant VM could not be read."
	}
	return st, ""
}

func (a *API) tenantApps(w http.ResponseWriter, r *http.Request) {
	c, ok := a.appsCaller(w, r)
	if !ok {
		return
	}
	t, ok := c.tenant(w, r.PathValue("tenantId"))
	if !ok {
		return
	}
	st, detail := a.readApps(r, t.ID)
	writeJSON(w, map[string]any{
		"signed_in":     "unknown",
		"desktop_ready": st != nil && st.XRDPLoopback,
		"can_prepare":   c.canManage(t),
		"status_detail": detail,
		"kvm":           st != nil && st.KVM,
		"blocks":        apps.Blocks(t.ID, st, detail),
		"apps":          apps.Catalog(),
	})
}

// tenantAppCopy records that a member copied an official command. The clipboard write happens
// in the browser; a failed audit does not hide the command.
func (a *API) tenantAppCopy(w http.ResponseWriter, r *http.Request) {
	c, ok := a.appsCaller(w, r)
	if !ok {
		return
	}
	t, ok := c.tenant(w, r.PathValue("tenantId"))
	if !ok {
		return
	}
	name := r.PathValue("app")
	text, _, found := apps.CopyTarget(name)
	if !found {
		auth.Problem(w, http.StatusNotFound, "Unknown app.")
		return
	}
	if text == "" {
		auth.Problem(w, http.StatusBadRequest, "This app has no install command to copy.")
		return
	}
	detail := "tvm-" + t.ID + " " + name
	if !t.member() {
		if !a.auditUse(r, c, t, "", "apps.command_copied", detail) {
			log.Printf("audit apps.command_copied: grant audit failed for %s", t.ID)
		}
	} else if err := a.db.AddAudit(r.Context(), db.AuditEntry{
		TenantID: t.ID, TenantName: t.Name, Actor: c.id.Sub, ActorName: c.id.Name,
		Action: "apps.command_copied", Detail: detail, IP: clientIP(r),
	}); err != nil {
		log.Printf("audit apps.command_copied: %v", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) tenantDesktopPrepare(w http.ResponseWriter, r *http.Request) {
	c, ok := a.appsCaller(w, r)
	if !ok {
		return
	}
	t, ok := c.tenant(w, r.PathValue("tenantId"))
	if !ok {
		return
	}
	if !c.canManage(t) {
		auth.Problem(w, http.StatusForbidden, "Only an owner or admin can prepare the desktop.")
		return
	}
	if !emptyBody(w, r) {
		return
	}
	detail := "tvm-" + t.ID
	if !t.member() {
		if !a.auditUse(r, c, t, "", "apps.desktop_prepare", detail) {
			refuseUnaudited(w)
			return
		}
	} else if !a.auditFirst(w, r, db.AuditEntry{
		TenantID: t.ID, TenantName: t.Name, Actor: c.id.Sub, ActorName: c.id.Name,
		Action: "apps.desktop_prepare", Detail: detail, IP: clientIP(r),
	}) {
		return
	}
	a.prepareDesktop(w, t.ID)
}

func (a *API) prepareDesktop(w http.ResponseWriter, tenant string) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	if err := a.host.DesktopPrepare(ctx, tenant); err != nil {
		auth.Problem(w, http.StatusBadGateway, "Desktop prerequisites failed.")
		return
	}
	writeJSON(w, map[string]string{"status": "ok"})
}

func emptyBody(w http.ResponseWriter, r *http.Request) bool {
	b, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil || len(strings.TrimSpace(string(b))) > 0 {
		auth.Problem(w, http.StatusBadRequest, "This action takes no parameters.")
		return false
	}
	return true
}
