package api

import (
	"errors"
	"log"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

var roleKeyRe = regexp.MustCompile(`^staff\.[a-z][a-z0-9_]{1,30}$`)

type roleIn struct {
	Key         string   `json:"key"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	Reason      string   `json:"reason"`
}

func (in *roleIn) check(w http.ResponseWriter) bool {
	in.Name, in.Description, in.Permissions = strings.TrimSpace(in.Name), strings.TrimSpace(in.Description), uniq(in.Permissions)
	for _, p := range in.Permissions {
		if !knownPerm(p) {
			auth.Problem(w, http.StatusBadRequest, "Unknown permission "+p+".")
			return false
		}
	}
	if in.Name == "" || len(in.Name) > 60 || len(in.Description) > 300 {
		auth.Problem(w, http.StatusBadRequest, "A name (up to 60 characters) is required; descriptions are up to 300.")
		return false
	}
	slices.Sort(in.Permissions)
	return reasonOK(w, in.Reason)
}

func (in roleIn) role() db.StaffRole {
	return db.StaffRole{Key: in.Key, Name: in.Name, Description: in.Description, Permissions: in.Permissions}
}

func (a *API) roleByKey(r *http.Request, key string) (*db.StaffRole, error) {
	rs, err := a.store().StaffRoles(r.Context())
	if err != nil {
		return nil, err
	}
	for i := range rs {
		if rs[i].Key == key {
			return &rs[i], nil
		}
	}
	return nil, nil
}

// teamCreateRole adds a role (audited with the DB write), then creates it in Nomen.
func (a *API) teamCreateRole(w http.ResponseWriter, r *http.Request, c *caller) {
	var in roleIn
	if !decodeStrict(w, r, &in) || !in.check(w) {
		return
	}
	if !roleKeyRe.MatchString(in.Key) {
		auth.Problem(w, http.StatusBadRequest, "Role keys look like staff.name (lowercase letters, digits, underscores).")
		return
	}
	if !in.granularOK(w) {
		return
	}
	e := teamEntry(r, c, "team.role_create", in.Key+" \""+in.Name+"\" ["+strings.Join(in.Permissions, ", ")+"] · "+in.Reason)
	if err := a.store().SaveStaffRole(r.Context(), in.role(), true, e); errors.Is(err, db.ErrNotFound) {
		auth.Problem(w, http.StatusConflict, "A role with that key already exists.")
		return
	} else if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The role couldn't be saved.")
		return
	}
	if err := a.idp().PutRole(r.Context(), in.Key, in.Name, false); err != nil {
		log.Printf("team role create %s: %v", in.Key, err)
		_ = a.store().DeleteStaffRole(r.Context(), in.Key, teamEntry(r, c, "team.role_sync_failed", in.Key+": "+a.cfg.IDP.Name+" refused; role removed again"))
		auth.Problem(w, http.StatusBadGateway, a.cfg.IDP.Name+" refused the role; nothing was created.")
		return
	}
	a.staffCache.flush()
	writeStatus(w, http.StatusCreated, in.role())
}

// teamUpdateRole renames or re-permissions a role. No self-grant (you can't add permissions to
// a role you hold), the last Owner stays, and with two-person on high-risk additions wait.
func (a *API) teamUpdateRole(w http.ResponseWriter, r *http.Request, c *caller) {
	var in roleIn
	in.Key = r.PathValue("roleKey")
	if !decodeStrict(w, r, &in) || !in.check(w) {
		return
	}
	in.Key = r.PathValue("roleKey")
	if !in.granularOK(w) {
		return
	}
	old, err := a.roleByKey(r, in.Key)
	if err != nil || old == nil {
		auth.Problem(w, http.StatusNotFound, "Role not found.")
		return
	}
	added := slices.DeleteFunc(slices.Clone(in.Permissions), func(p string) bool { return slices.Contains(old.Permissions, p) })
	if len(added) > 0 && slices.Contains(a.staffOf(r.Context(), c).Roles, in.Key) {
		a.auditOnce(r, c, "authz.denied", "team.roles", "self-grant refused")
		auth.Problem(w, http.StatusForbidden, "You hold this role, so you can't add permissions to it; ask another Owner.")
		return
	}
	next := in.role()
	if n, err := a.ownersAfter(r.Context(), nil, &next, ""); err != nil || n == 0 {
		auth.Problem(w, http.StatusConflict, "That would leave no active Owner (team.write).")
		return
	}
	if a.twoPersonFor(r, toSet(added)) {
		a.queueRequest(w, r, c, "role", in.Key, in.Name, in)
		return
	}
	a.applyRoleUpdate(w, r, c, *old, in)
}

func (a *API) applyRoleUpdate(w http.ResponseWriter, r *http.Request, c *caller, old db.StaffRole, in roleIn) {
	e := teamEntry(r, c, "team.role_update", in.Key+" \""+old.Name+"\" → \""+in.Name+"\"; ["+strings.Join(old.Permissions, ", ")+"] → ["+strings.Join(in.Permissions, ", ")+"] · "+in.Reason)
	if err := a.store().SaveStaffRole(r.Context(), in.role(), false, e); err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The role couldn't be saved.")
		return
	}
	if old.Name != in.Name {
		if err := a.idp().PutRole(r.Context(), in.Key, in.Name, true); err != nil {
			log.Printf("team role update %s: %v", in.Key, err)
			_ = a.store().RestoreStaffRole(r.Context(), old, teamEntry(r, c, "team.role_sync_failed", in.Key+": "+a.cfg.IDP.Name+" refused; previous version restored"))
			auth.Problem(w, http.StatusBadGateway, a.cfg.IDP.Name+" refused the rename; the role is unchanged.")
			return
		}
	}
	a.staffCache.flush()
	writeJSON(w, in.role())
}
