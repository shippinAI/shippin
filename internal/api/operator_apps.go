package api

import (
	"net/http"

	"shippin.cloud/console/internal/apps"
	"shippin.cloud/console/internal/db"
)

func (a *API) opTenantApps(w http.ResponseWriter, r *http.Request, _ *caller) {
	t, ok := a.opTenant(w, r)
	if !ok {
		return
	}
	st, detail := a.readApps(r, t.ID)
	writeJSON(w, map[string]any{
		"signed_in":     "unknown",
		"desktop_ready": st != nil && st.XRDPLoopback,
		"status_detail": detail,
		"apps":          apps.States(st),
	})
}

func (a *API) opDesktopPrepare(w http.ResponseWriter, r *http.Request, c *caller) {
	t, ok := a.opTenant(w, r)
	if !ok {
		return
	}
	if !emptyBody(w, r) {
		return
	}
	if !a.auditFirst(w, r, db.AuditEntry{
		TenantID: t.ID, TenantName: t.Name, Actor: c.id.Sub, ActorName: c.id.Name,
		Action: "apps.desktop_prepare", Detail: "tvm-" + t.ID, IP: clientIP(r),
	}) {
		return
	}
	a.prepareDesktop(w, t.ID)
}
