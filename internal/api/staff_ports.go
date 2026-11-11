package api

import (
	"context"
	"shippin.cloud/console/internal/devaccounts"
	"time"

	"shippin.cloud/console/internal/db"
)

// staffStore is the Postgres side of staff authorization (roles, settings, audit). *db.DB
// implements it; tests substitute a fake.
type staffStore interface {
	StaffRoles(ctx context.Context) ([]db.StaffRole, error)
	SaveStaffRole(ctx context.Context, r db.StaffRole, create bool, es ...db.AuditEntry) error
	DeleteStaffRole(ctx context.Context, key string, es ...db.AuditEntry) error
	RestoreStaffRole(ctx context.Context, r db.StaffRole, es ...db.AuditEntry) error
	StaffSettings(ctx context.Context) (db.StaffSettings, error)
	SaveStaffSettings(ctx context.Context, s db.StaffSettings, by string, es ...db.AuditEntry) error
	TouchStaff(ctx context.Context, user string) error
	StaffSeen(ctx context.Context) (map[string]time.Time, error)
	CreateStaffRequest(ctx context.Context, q db.StaffRequest, es ...db.AuditEntry) error
	StaffRequests(ctx context.Context) ([]db.StaffRequest, error)
	StaffRequestByID(ctx context.Context, id string) (*db.StaffRequest, error)
	DecideStaffRequest(ctx context.Context, id, status, by string, es ...db.AuditEntry) error
	AddAudit(ctx context.Context, e db.AuditEntry) error
}

// staffGrant is a user's authorization on the staff project, granted by the operator org.
type staffGrant struct {
	ID    string
	Roles []string
}

// staffUser is a user of the operator org as the Team page shows it.
type staffUser struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Login string `json:"login"`
	State string `json:"state"` // active | inactive | initial | locked
	Kind  string `json:"kind"`  // human | machine
}

// staffIdP is the Nomen side: grants, MFA state, users and project roles. Writes use the
// console-team machine user only.
type staffIdP interface {
	Grant(ctx context.Context, user string) (staffGrant, error)
	AllGrants(ctx context.Context) (map[string]staffGrant, error)
	MFA(ctx context.Context, user string) (bool, error)
	Users(ctx context.Context) ([]staffUser, error)
	User(ctx context.Context, id string) (*staffUser, error)
	Invite(ctx context.Context, email, given, family string) (string, error)
	SetRoles(ctx context.Context, user string, g staffGrant, roles []string) error
	SetActive(ctx context.Context, user string, active bool) error
	RoleKeys(ctx context.Context) ([]string, error)
	PutRole(ctx context.Context, key, name string, exists bool) error
	DeleteRole(ctx context.Context, key string) error
}

func (a *API) store() staffStore {
	if a.staffDB != nil {
		return a.staffDB
	}
	return a.db
}

// localRoles says this panel holds staff roles itself: nothing has been set up to hold them elsewhere.
func (a *API) localRoles() bool {
	return a.staffIdP == nil && a.db != nil && a.cfg.Staff.TeamToken == "" && a.cfg.IDP.ClientID == ""
}

func (a *API) idp() staffIdP {
	if a.staffIdP != nil {
		return a.staffIdP
	}
	if a.localRoles() {
		return &localStaff{db: a.db, noSecondFactor: func(ctx context.Context) bool { return a.teamSettings(ctx).NoSecondFactor }, devStaff: devaccounts.ListedStaff}
	}
	return &nomenStaff{flow: a.flow, tok: a.cfg.Staff.TeamToken, org: a.cfg.StaffOrg(), project: a.cfg.Staff.ProjectID}
}
