package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

// permGrantStore keeps direct (per-principal) grants of grantable permissions, cloud-IAM style.
// *db.DB implements it; tests substitute a fake.
type permGrantStore interface {
	PermGrants(ctx context.Context) ([]db.PermGrant, error)
	PermGrantsOf(ctx context.Context, subject string) ([]db.PermGrant, error)
	GrantPerm(ctx context.Context, g db.PermGrant, es ...db.AuditEntry) error
	RevokePerm(ctx context.Context, subject, perm string, es ...db.AuditEntry) error
}

// noGrants is the store when no database is wired (gate tests on fakes): nobody holds a grant.
type noGrants struct{}

func (noGrants) PermGrants(context.Context) ([]db.PermGrant, error)           { return []db.PermGrant{}, nil }
func (noGrants) PermGrantsOf(context.Context, string) ([]db.PermGrant, error) { return nil, nil }
func (noGrants) GrantPerm(context.Context, db.PermGrant, ...db.AuditEntry) error {
	return errors.New("no grant store")
}
func (noGrants) RevokePerm(context.Context, string, string, ...db.AuditEntry) error {
	return errors.New("no grant store")
}

func (a *API) grants() permGrantStore {
	switch {
	case a.permDB != nil:
		return a.permDB
	case a.staffDB != nil || a.db == nil:
		return noGrants{}
	}
	return a.db
}

// grantRoutes: listing needs team.read; granting and revoking need team.write (Owner). Every
// change is audited in the same transaction as the write and applies on the next request.
func (a *API) grantRoutes() []opRoute {
	return []opRoute{
		{"GET /v1/operator/team/permission-grants", "team.read", a.grantList},
		{"POST /v1/operator/team/permission-grants", "team.write", a.grantAdd},
		{"DELETE /v1/operator/team/permission-grants/{userId}/{permKey}", "team.write", a.grantRevoke},
	}
}

func (a *API) grantList(w http.ResponseWriter, r *http.Request, _ *caller) {
	gs, err := a.grants().PermGrants(r.Context())
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Direct grants are unavailable right now.")
		return
	}
	perms := []Perm{}
	for _, p := range permCatalog {
		if p.Grantable {
			perms = append(perms, p)
		}
	}
	writeJSON(w, map[string]any{"items": gs, "grantable": perms})
}

type grantIn struct {
	Subject    string `json:"subject"`
	Permission string `json:"permission"`
	Reason     string `json:"reason"`
}

func (a *API) grantAdd(w http.ResponseWriter, r *http.Request, c *caller) {
	var in grantIn
	if !decodeStrict(w, r, &in) {
		return
	}
	in.Subject, in.Reason = strings.TrimSpace(in.Subject), strings.TrimSpace(in.Reason)
	if !grantablePerm(in.Permission) {
		auth.Problem(w, http.StatusBadRequest, "Only the granular catalog permissions can be granted directly; use a role for the rest.")
		return
	}
	if !reasonOK(w, in.Reason) {
		return
	}
	if in.Subject == "" || in.Subject == c.id.Sub {
		auth.Problem(w, http.StatusBadRequest, "Pick another team member (you can't grant yourself).")
		return
	}
	u, err := a.idp().User(r.Context(), in.Subject)
	if err != nil || u == nil {
		auth.Problem(w, http.StatusNotFound, "No such member of the operator org.")
		return
	}
	e := teamEntry(r, c, "team.perm_grant", in.Permission+" -> "+u.ID+" \""+u.Name+"\" · "+in.Reason)
	err = a.grants().GrantPerm(r.Context(), db.PermGrant{Subject: u.ID, Permission: in.Permission, Reason: in.Reason, GrantedBy: c.id.Sub}, e)
	switch {
	case errors.Is(err, db.ErrNotFound):
		auth.Problem(w, http.StatusConflict, "That member already holds this grant.")
		return
	case err != nil:
		auth.Problem(w, http.StatusServiceUnavailable, "The grant couldn't be saved or audited, so it wasn't made.")
		return
	}
	a.staffCache.flush()
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"status":"granted"}`))
}

func (a *API) grantRevoke(w http.ResponseWriter, r *http.Request, c *caller) {
	sub, perm := r.PathValue("userId"), r.PathValue("permKey")
	reason := strings.TrimSpace(r.URL.Query().Get("reason"))
	if !reasonOK(w, reason) {
		return
	}
	e := teamEntry(r, c, "team.perm_revoke", perm+" -> "+sub+" · "+reason)
	err := a.grants().RevokePerm(r.Context(), sub, perm, e)
	switch {
	case errors.Is(err, db.ErrNotFound):
		auth.Problem(w, http.StatusNotFound, "No such grant.")
		return
	case err != nil:
		auth.Problem(w, http.StatusServiceUnavailable, "The grant couldn't be revoked or audited; it is unchanged.")
		return
	}
	a.staffCache.flush()
	w.WriteHeader(http.StatusNoContent)
}
