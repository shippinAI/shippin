package api

import (
	"net/http/httptest"
	"testing"

	"shippin.cloud/console/internal/db"
)

func TestTwoPersonRule(t *testing.T) {
	s, idp, a := ownerTeam()
	s.settings.TwoPerson = true
	// One Owner: the rule can't apply, the change goes through.
	if c, _ := teamCall(a, "PUT", "/v1/operator/team/members/hal/roles", "jo", `{"roles":["staff.security"],`+why+`}`); c != 200 {
		t.Fatalf("single Owner: %d, want 200", c)
	}
	idp.add("kim", "staff.owner")
	c, body := teamCall(a, "PUT", "/v1/operator/team/members/sam/roles", "jo", `{"roles":["staff.owner"],`+why+`}`)
	if c != 202 || body["status"] != "pending_approval" {
		t.Fatalf("high-risk grant with two Owners: %d %v", c, body)
	}
	id := body["id"].(string)
	if c, _ := teamCall(a, "POST", "/v1/operator/team/requests/"+id+"/approve", "jo", ``); c != 403 {
		t.Fatalf("requester approving: %d, want 403", c)
	}
	if c, _ := teamCall(a, "POST", "/v1/operator/team/requests/"+id+"/approve", "kim", ``); c != 200 {
		t.Fatalf("second Owner approving: %d, want 200", c)
	}
	if g := idp.grants["sam"]; len(g.Roles) != 1 || g.Roles[0] != "staff.owner" {
		t.Fatalf("approved change not applied: %v", g)
	}
	// Low-risk grants never wait.
	if c, _ := teamCall(a, "PUT", "/v1/operator/team/members/hal/roles", "jo", `{"roles":["staff.security","staff.auditor"],`+why+`}`); c != 200 {
		t.Fatalf("low-risk grant: %d", c)
	}
	// A request for your own roles can't be approved by you.
	_, body = teamCall(a, "PUT", "/v1/operator/team/members/kim/roles", "jo", `{"roles":["staff.owner","staff.ops"],`+why+`}`)
	if c, _ := teamCall(a, "POST", "/v1/operator/team/requests/"+body["id"].(string)+"/approve", "kim", ``); c != 403 {
		t.Fatalf("target approving own change: %d, want 403", c)
	}
}

func TestSettingsValidatedAndAudited(t *testing.T) {
	s, idp, a := ownerTeam()
	for _, b := range []string{
		`{"mode":"yolo","idle_days":90,"approval_minutes":30,` + why + `}`,
		`{"mode":"enforce","idle_days":0,"approval_minutes":30,` + why + `}`,
		`{"mode":"enforce","idle_days":90,"approval_minutes":1,` + why + `}`,
	} {
		if c, _ := teamCall(a, "PUT", "/v1/operator/team/settings", "jo", b); c != 400 {
			t.Errorf("%s: %d, want 400", b, c)
		}
	}
	idp.mfa["jo"] = true
	ok := `{"mode":"enforce","legacy_fallback":false,"two_person":true,"idle_days":30,"breakglass_approval":true,"approval_minutes":15,"sole_owner_activation":false,` + why + `}`
	if c, _ := teamCall(a, "PUT", "/v1/operator/team/settings", "jo", ok); c != 200 || s.settings.IdleDays != 30 || !s.settings.TwoPerson {
		t.Fatalf("save: %d %+v", c, s.settings)
	}
	if c, _ := teamCall(a, "PUT", "/v1/operator/team/settings", "sam", ok); c != 403 {
		t.Fatalf("support changing settings: %d", c)
	}
	// No Owner has MFA: enforcing without the fallback would lock everyone out.
	s.roles = append(s.roles, db.StaffRole{Key: "staff.cfg", Name: "Settings", Permissions: []string{"team.settings"}})
	idp.add("cfg", "staff.cfg")
	idp.mfa["jo"] = false
	a.staffCache.flush()
	if c, _ := teamCall(a, "PUT", "/v1/operator/team/settings", "cfg", ok); c != 409 {
		t.Fatalf("enforce without fallback and no Owner with MFA: %d, want 409", c)
	}
}

func TestBreakGlassPath(t *testing.T) {
	r := httptest.NewRequest("POST", "/v1/operator/break-glass", nil)
	s, idp, a := ownerTeam()
	if p, _ := a.breakGlassPath(r, &caller{id: testID("jo")}); p != "sole_owner" {
		t.Fatalf("only Owner, nobody else to approve: %q", p)
	}
	s.settings.SoleOwnerActivation = false
	a.staffCache.flush()
	if p, msg := a.breakGlassPath(r, &caller{id: testID("jo")}); p != "" || msg == "" {
		t.Fatalf("sole activation off: %q", p)
	}
	idp.add("sec", "staff.security")
	if p, _ := a.breakGlassPath(r, &caller{id: testID("jo")}); p != "approval" {
		t.Fatalf("an approver exists: %q", p)
	}
	if p, _ := a.breakGlassPath(r, &caller{id: testID("sec")}); p != "approval" {
		t.Fatalf("the Owner approves the other's request (never the requester's own): %q", p)
	}
	idp.mfa["sec"] = false
	if p, _ := a.breakGlassPath(r, &caller{id: testID("jo")}); p == "approval" {
		t.Fatal("an approver without MFA doesn't count")
	}
	s.settings.BreakGlassApproval = false
	a.staffCache.flush()
	if p, _ := a.breakGlassPath(r, &caller{id: testID("jo")}); p != "direct" {
		t.Fatalf("approval off: %q", p)
	}
}
