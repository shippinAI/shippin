package api

import (
	"context"
	"net/http"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/db"
)

// bgAPI: real Postgres for grants and audit, fakes for staff roles and Nomen, fresh sign-in.
func bgAPI(t *testing.T, idp *fakeIdP) (*API, *db.DB, db.Tenant) {
	t.Helper()
	url := os.Getenv("SHIPPIN_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("SHIPPIN_TEST_DATABASE_URL not set")
	}
	d, err := db.Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Pool.Close)
	id, _ := uuid.NewV7()
	org, _ := uuid.NewV7()
	tn := db.Tenant{ID: id.String(), IdpOrgID: org.String(), Name: "BG Co", Plan: "p"}
	if err := d.CreateTenant(context.Background(), tn); err != nil {
		t.Fatal(err)
	}
	a := staffAPI(newFakeStore("enforce"), idp)
	a.db = d
	a.authAt = func(*http.Request) time.Time { return time.Now().Add(-time.Minute) }
	return a, d, tn
}

func bgBody(tn db.Tenant) string {
	return `{"tenant_id":"` + tn.ID + `","category":"security_incident","justification":"Live incident on the tenant edge, need logs now"}`
}

func auditActions(t *testing.T, d *db.DB, grant string) []string {
	es, err := d.AllAudit(context.Background(), "", 500)
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, e := range es {
		if e.GrantID == grant {
			out = append(out, e.Action)
		}
	}
	return out
}

func TestBreakGlassNeedsAnotherApprover(t *testing.T) {
	a, d, tn := bgAPI(t, newFakeIdP().add("jo", "staff.owner").add("ops", "staff.ops").add("sec", "staff.security"))
	c, body := teamCall(a, "POST", "/v1/operator/break-glass", "ops", bgBody(tn))
	if c != 202 || body["status"] != "pending" {
		t.Fatalf("with approvers around, break-glass waits: %d %v", c, body)
	}
	id := body["id"].(string)
	if c, _ := teamCall(a, "POST", "/v1/operator/break-glass/"+id+"/approve", "ops", ``); c != 403 {
		t.Fatalf("requester can't approve (no self-approval, and ops lacks the permission): %d", c)
	}
	if c, _ := teamCall(a, "POST", "/v1/operator/break-glass/"+id+"/approve", "sec", ``); c != 200 {
		t.Fatalf("security approves: %d", c)
	}
	g, _ := d.GrantByID(context.Background(), id)
	if g.Status != "approved" || g.DecidedBy != "sec" || !g.Active(time.Now()) {
		t.Fatalf("grant after approval: %+v", g)
	}
	if acts := auditActions(t, d, id); !slices.Contains(acts, "breakglass.request") || !slices.Contains(acts, "breakglass.approve") {
		t.Fatalf("audit: %v", acts)
	}
	// An Owner who requests can't approve their own either, even holding the permission.
	_, body = teamCall(a, "POST", "/v1/operator/break-glass", "jo", bgBody(tn))
	if c, _ := teamCall(a, "POST", "/v1/operator/break-glass/"+body["id"].(string)+"/approve", "jo", ``); c != 403 {
		t.Fatalf("Owner self-approval: %d, want 403", c)
	}
	a.authAt = func(*http.Request) time.Time { return time.Time{} }
	if c, _ := teamCall(a, "POST", "/v1/operator/break-glass/"+body["id"].(string)+"/approve", "sec", ``); c != 428 {
		t.Fatalf("approver without a fresh sign-in: %d, want 428", c)
	}
}

func TestBreakGlassSoleOwnerIsFlagged(t *testing.T) {
	a, d, tn := bgAPI(t, newFakeIdP().add("jo", "staff.owner").add("ops", "staff.ops"))
	c, body := teamCall(a, "POST", "/v1/operator/break-glass", "jo", bgBody(tn))
	if c != 201 || body["status"] != "approved" {
		t.Fatalf("sole Owner activation: %d %v", c, body)
	}
	if acts := auditActions(t, d, body["id"].(string)); !slices.Contains(acts, "breakglass.unapproved") {
		t.Fatalf("sole-Owner activation must be flagged: %v", acts)
	}
	if c, _ := teamCall(a, "POST", "/v1/operator/break-glass", "ops", bgBody(tn)); c != 202 {
		t.Fatalf("ops with the Owner as approver waits: %d, want 202", c)
	}
	b, _, tn2 := bgAPI(t, newFakeIdP().add("ops", "staff.ops"))
	if c, _ := teamCall(b, "POST", "/v1/operator/break-glass", "ops", bgBody(tn2)); c != 409 {
		t.Fatalf("non-Owner with nobody to approve: %d, want 409", c)
	}
	withhold := `{"tenant_id":"` + tn.ID + `","category":"legal_hold","withhold_notice":true,"justification":"Court order, notice delayed by counsel"}`
	if c, _ := teamCall(a, "POST", "/v1/operator/break-glass", "ops", withhold); c != 403 {
		t.Fatalf("legal hold without access.legal_hold: %d, want 403", c)
	}
}
