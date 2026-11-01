package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

// twoPersonFor: the two-person rule applies when it's on, there are 2+ active Owners and the
// change grants a high-risk permission.
func (a *API) twoPersonFor(r *http.Request, granted map[string]bool) bool {
	if !a.teamSettings(r.Context()).TwoPerson {
		return false
	}
	keys := []string{}
	for k := range granted {
		keys = append(keys, k)
	}
	if !highRisk(keys) {
		return false
	}
	n, err := a.ownersAfter(r.Context(), nil, nil, "")
	return err != nil || n >= 2 // can't count: fail closed (needs a second Owner)
}

// queueRequest stores a pending two-person request (audited) instead of applying the change.
func (a *API) queueRequest(w http.ResponseWriter, r *http.Request, c *caller, kind, target, name string, payload any) {
	id, _ := uuid.NewV7()
	q := db.StaffRequest{ID: id.String(), Kind: kind, Target: target, TargetName: name, Payload: mustJSON(payload),
		Reason: reasonOf(payload), RequestedBy: c.id.Sub, RequestedName: displayName(c)}
	e := teamEntry(r, c, "team.request", kind+" for "+name+" waits for a second Owner ("+q.ID+")")
	if err := a.store().CreateStaffRequest(r.Context(), q, e); err != nil {
		log.Printf("team request: %v", err)
		auth.Problem(w, http.StatusServiceUnavailable, "The request couldn't be saved.")
		return
	}
	writeStatus(w, http.StatusAccepted, map[string]string{"id": q.ID, "status": "pending_approval"})
}

func reasonOf(p any) string {
	switch v := p.(type) {
	case rolesIn:
		return v.Reason
	case inviteIn:
		return v.Reason
	case roleIn:
		return v.Reason
	}
	return ""
}

// teamDecideRequest: approve or reject someone else's request (the requester may only cancel).
// Approval re-checks every invariant against the current state before applying.
func (a *API) teamDecideRequest(w http.ResponseWriter, r *http.Request, c *caller) {
	d := r.PathValue("decision")
	q, err := a.store().StaffRequestByID(r.Context(), r.PathValue("requestId"))
	if err != nil || q == nil || q.Status != "pending" {
		auth.Problem(w, http.StatusNotFound, "Request not found or already decided.")
		return
	}
	status := map[string]string{"approve": "approved", "reject": "rejected", "cancel": "cancelled"}[d]
	switch {
	case status == "":
		auth.Problem(w, http.StatusNotFound, "Unknown decision.")
		return
	case (d == "cancel") != (q.RequestedBy == c.id.Sub):
		auth.Problem(w, http.StatusForbidden, "Only someone else can approve or reject; the requester can only cancel.")
		return
	case d == "approve" && q.Target == c.id.Sub:
		auth.Problem(w, http.StatusForbidden, "You can't approve a change to your own roles.")
		return
	}
	e := teamEntry(r, c, "team.request_"+status, q.Kind+" for "+q.TargetName+" ("+q.ID+")")
	if err := a.store().DecideStaffRequest(r.Context(), q.ID, status, c.id.Sub, e); err != nil {
		auth.Problem(w, http.StatusConflict, "This request changed meanwhile; refresh.")
		return
	}
	if d != "approve" {
		writeJSON(w, map[string]string{"id": q.ID, "status": status})
		return
	}
	switch q.Kind {
	case "roles":
		var in rolesIn
		_ = json.Unmarshal(q.Payload, &in)
		cur, err := a.idp().Grant(r.Context(), q.Target)
		if err != nil {
			auth.Problem(w, http.StatusBadGateway, a.cfg.IDP.Name+" couldn't be read.")
			return
		}
		if n, err := a.ownersAfter(r.Context(), map[string][]string{q.Target: in.Roles}, nil, ""); err != nil || n == 0 {
			auth.Problem(w, http.StatusConflict, "Approved, but applying it now would leave no active Owner; nothing changed.")
			return
		}
		in.Reason = strings.TrimSpace(in.Reason + " (approved by " + displayName(c) + ")")
		a.applyRoles(w, r, c, q.Target, q.TargetName, cur, in)
	case "role":
		var in roleIn
		_ = json.Unmarshal(q.Payload, &in)
		old, err := a.roleByKey(r, q.Target)
		next := in.role()
		if err != nil || old == nil {
			auth.Problem(w, http.StatusConflict, "Approved, but the role no longer exists; nothing changed.")
			return
		}
		if n, err := a.ownersAfter(r.Context(), nil, &next, ""); err != nil || n == 0 {
			auth.Problem(w, http.StatusConflict, "Approved, but applying it now would leave no active Owner; nothing changed.")
			return
		}
		a.applyRoleUpdate(w, r, c, *old, in)
	case "invite":
		var in inviteIn
		_ = json.Unmarshal(q.Payload, &in)
		a.applyInvite(w, r, c, in)
	}
}
