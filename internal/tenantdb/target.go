// Package tenantdb is the console's view of a tenant's starter Postgres.
//
// Every session is opened as that tenant's own LOGIN role (tn_*) on that tenant's
// database (tdb_*) only. The names match hostd starterNames. The vaultix database
// and vx_* schemas are not user databases and are never opened or listed.
package tenantdb

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	// identRe is an unquoted Postgres identifier we are willing to interpolate.
	identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,62}$`)
	tdbRe   = regexp.MustCompile(`(?i)^tdb_[0-9a-f]{32}$`)
	tnRe    = regexp.MustCompile(`(?i)^tn_[0-9a-f]{32}$`)
	vxRe    = regexp.MustCompile(`(?i)^vx_[A-Za-z0-9_]*$`)
)

// Target is the only database a tenant's console session may touch.
type Target struct {
	TenantID string
	Database string
	Role     string
}

// Names returns the starter database and role for a tenant UUID.
// It is the same formula as hostd starterNames: strip dashes, prefix tdb_ / tn_.
func Names(tenantID string) (database, role string, err error) {
	if !uuidRe.MatchString(tenantID) {
		return "", "", fmt.Errorf("invalid tenant id")
	}
	h := strings.ReplaceAll(tenantID, "-", "")
	return "tdb_" + h, "tn_" + h, nil
}

// TargetOf resolves a tenant id into the only database this package will open.
func TargetOf(tenantID string) (Target, error) {
	db, role, err := Names(tenantID)
	if err != nil {
		return Target{}, err
	}
	t := Target{TenantID: tenantID, Database: db, Role: role}
	if err := t.Check(); err != nil {
		return Target{}, err
	}
	return t, nil
}

// Check rejects anything that is not this tenant's starter database.
func (t Target) Check() error {
	if t.Database == "" || t.Role == "" || strings.EqualFold(t.Database, "vaultix") {
		return ErrForbiddenDatabase
	}
	if !tdbRe.MatchString(t.Database) || !tnRe.MatchString(t.Role) {
		return ErrForbiddenDatabase
	}
	wantDB, wantRole, err := Names(t.TenantID)
	if err != nil || !strings.EqualFold(t.Database, wantDB) || !strings.EqualFold(t.Role, wantRole) {
		return ErrForbiddenDatabase
	}
	return nil
}

// VisibleSchema reports whether a schema may appear in the user's database UI.
// pg catalogs and Vaultix schemas (vx_*) are hidden.
func VisibleSchema(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "", "pg_catalog", "information_schema", "pg_toast", "vaultix":
		return false
	}
	if strings.HasPrefix(n, "pg_") || vxRe.MatchString(n) {
		return false
	}
	return identRe.MatchString(name)
}

// SafeIdent accepts a single SQL identifier (table, column, schema).
func SafeIdent(name string) bool {
	return identRe.MatchString(name) && !vxRe.MatchString(name) && !strings.EqualFold(name, "vaultix")
}

// PlatformTable marks the starter tables hostd creates for platform services.
// They are real tables in the tenant database, not Vaultix storage.
func PlatformTable(name string) bool {
	switch name {
	case "secrets_refs", "idp_users", "idp_groups", "dns_zones", "dns_records":
		return true
	default:
		return false
	}
}

// Guard is applied to every session after the role check.
type Guard struct {
	Timeout  time.Duration
	ReadOnly bool
	MaxRows  int
}

func (g Guard) withDefaults() Guard {
	if g.Timeout <= 0 {
		g.Timeout = 8 * time.Second
	}
	if g.Timeout > 30*time.Second {
		g.Timeout = 30 * time.Second
	}
	if g.MaxRows <= 0 {
		g.MaxRows = 200
	}
	if g.MaxRows > 1000 {
		g.MaxRows = 1000
	}
	return g
}

// Identity is what the session reports about itself. Both users must be the
// tenant role and neither may be a superuser.
type Identity struct {
	SessionUser  string
	User         string
	Database     string
	SessionSuper bool
	Super        bool
}

var (
	ErrForbiddenDatabase = errors.New("that database is not available in the console")
	ErrSuperuser         = errors.New("refusing to run as a superuser")
	ErrRoleMismatch      = errors.New("session is not the tenant database role")
	ErrDatabaseMismatch  = errors.New("session is not the tenant database")
	ErrNotConfigured     = errors.New("tenant database login is not configured")
	ErrReadOnly          = errors.New("read-only mode is on")
	ErrAudit             = errors.New("the query was not run because the audit log is unavailable")
)

// Authorize fails closed unless the live session is exactly the tenant role
// on the tenant database, and is not a superuser.
func Authorize(t Target, id Identity) error {
	if err := t.Check(); err != nil {
		return err
	}
	if id.Super || id.SessionSuper {
		return ErrSuperuser
	}
	if !strings.EqualFold(id.SessionUser, t.Role) || !strings.EqualFold(id.User, t.Role) {
		return ErrRoleMismatch
	}
	if strings.EqualFold(id.SessionUser, "postgres") || strings.EqualFold(id.User, "postgres") {
		return ErrSuperuser
	}
	if !strings.EqualFold(id.Database, t.Database) || strings.EqualFold(id.Database, "vaultix") {
		return ErrDatabaseMismatch
	}
	return nil
}
