package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"shippin.cloud/console/internal/auth"
)

// opDeleteTenant tears down a tenant. The caller must type the tenant's exact name; the
// hostd teardown removes workspaces, then runs the provisioning rollback (network, peers,
// forwards) and drops the project and pool. The DB rows are retired, not deleted.
func (a *API) opDeleteTenant(w http.ResponseWriter, r *http.Request, c *caller) {
	t, ok := a.opTenant(w, r)
	if !ok {
		return
	}
	var body struct {
		Confirm string `json:"confirm"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body) != nil || body.Confirm != t.Name {
		auth.Problem(w, http.StatusBadRequest, "Type the tenant name exactly to confirm deletion.")
		return
	}
	if !a.auditOperator(w, r, c, t.ID, "tenant.delete", t.Status) {
		return
	}
	won, err := a.db.BeginTenantOp(r.Context(), t.ID, "deleting", "active", "suspended", "failed", "provisioning")
	if err != nil || !won {
		auth.Problem(w, http.StatusConflict, "Tenant is "+t.Status+"; it can't be deleted right now.")
		return
	}
	log.Printf("operator %s deleting tenant %s (%s)", c.id.Sub, t.ID, t.Name)
	// Detached from the request: a closed browser tab must not abort a half-done teardown.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	ds, err := a.host.DeleteTenant(ctx, t.ID)
	if err != nil {
		log.Printf("delete tenant %s: %v", t.ID, err)
		_ = a.db.SetTenantStatus(ctx, t.ID, "failed", t.SSHIP, "Delete failed: "+err.Error())
		auth.Problem(w, http.StatusBadGateway, "Delete failed: "+err.Error())
		return
	}
	if err := a.db.MarkTenantDeleted(ctx, t.ID); err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Host resources were removed but the tenant row could not be updated.")
		return
	}
	writeJSON(w, map[string]string{"tenant_id": t.ID, "status": "deleted", "dataset_kept": ds})
}
