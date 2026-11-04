package api

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"shippin.cloud/console/internal/db"
)

// localStaff keeps staff roles in the panel's own database, over the accounts it already has. It is
// what an installation uses unless its identity provider is set up to hold roles, so roles work with
// any way of signing in, including none but the panel's own.
type localStaff struct {
	db *db.DB
	// noSecondFactor is the team setting that lets staff work without one, for a site that cannot
	// offer passkeys.
	noSecondFactor func(context.Context) bool
}

func localID(sub string) (string, bool) {
	id, ok := strings.CutPrefix(sub, localSubPrefix)
	return id, ok && uuid.Validate(id) == nil
}

const ownerRoleKey = "staff.owner"

func grantOf(sub, role string, active bool, roles []string) staffGrant {
	if !active {
		return staffGrant{ID: sub}
	}
	out := append([]string{}, roles...)
	// The installation's owner always holds the Owner role: nothing else can give it to them.
	if role == "owner" && !containsString(out, ownerRoleKey) {
		out = append(out, ownerRoleKey)
	}
	return staffGrant{ID: sub, Roles: out}
}

func containsString(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// Grant is one person's roles. A subject from elsewhere has none here.
func (s *localStaff) Grant(ctx context.Context, user string) (staffGrant, error) {
	id, ok := localID(user)
	if !ok {
		return staffGrant{}, nil
	}
	var role string
	var active bool
	var roles []string
	err := s.db.Pool.QueryRow(ctx, `SELECT u.role, u.active, coalesce(a.roles, '{}') FROM identity_users u
		LEFT JOIN local_staff_roles a ON a.user_id = u.id WHERE u.id = $1`, id).Scan(&role, &active, &roles)
	if errors.Is(err, pgx.ErrNoRows) {
		return staffGrant{}, nil
	}
	if err != nil {
		return staffGrant{}, err
	}
	return grantOf(user, role, active, roles), nil
}

func (s *localStaff) AllGrants(ctx context.Context) (map[string]staffGrant, error) {
	rows, err := s.db.Pool.Query(ctx, `SELECT u.id::text, u.role, u.active, coalesce(a.roles, '{}') FROM identity_users u
		LEFT JOIN local_staff_roles a ON a.user_id = u.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]staffGrant{}
	for rows.Next() {
		var id, role string
		var active bool
		var roles []string
		if err := rows.Scan(&id, &role, &active, &roles); err != nil {
			return nil, err
		}
		out[localSubPrefix+id] = grantOf(localSubPrefix+id, role, active, roles)
	}
	return out, rows.Err()
}

// MFA is a passkey, or signing in through an identity provider, whose own policy then applies. The
// owner set up this installation with a one-time code and is not locked out of it.
func (s *localStaff) MFA(ctx context.Context, user string) (bool, error) {
	id, ok := localID(user)
	if !ok {
		return false, nil
	}
	var owner, passkey, linked bool
	err := s.db.Pool.QueryRow(ctx, `SELECT u.role = 'owner',
		EXISTS (SELECT 1 FROM identity_passkeys p WHERE p.user_id = u.id),
		EXISTS (SELECT 1 FROM identity_links l WHERE l.user_id = u.id)
		FROM identity_users u WHERE u.id = $1`, id).Scan(&owner, &passkey, &linked)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return owner || passkey || linked || (s.noSecondFactor != nil && s.noSecondFactor(ctx)), nil
}

func localUserView(id, email, name string, active bool) staffUser {
	if name == "" {
		name = email
	}
	state := "active"
	if !active {
		state = "inactive"
	}
	return staffUser{ID: localSubPrefix + id, Name: name, Email: email, State: state, Kind: "human"}
}

func (s *localStaff) Users(ctx context.Context) ([]staffUser, error) {
	rows, err := s.db.Pool.Query(ctx, `SELECT id::text, email, name, active FROM identity_users ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []staffUser{}
	for rows.Next() {
		var id, email, name string
		var active bool
		if err := rows.Scan(&id, &email, &name, &active); err != nil {
			return nil, err
		}
		out = append(out, localUserView(id, email, name, active))
	}
	return out, rows.Err()
}

func (s *localStaff) User(ctx context.Context, sub string) (*staffUser, error) {
	id, ok := localID(sub)
	if !ok {
		return nil, nil
	}
	var email, name string
	var active bool
	err := s.db.Pool.QueryRow(ctx, `SELECT email, name, active FROM identity_users WHERE id = $1`, id).Scan(&email, &name, &active)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u := localUserView(id, email, name, active)
	return &u, nil
}

// joinOwnerOrg puts someone on the owner's org, so that the changes they ask for reach the owner's board.
const joinOwnerOrg = `INSERT INTO identity_members (org_id, user_id, role)
	SELECT m.org_id, $1, 'member' FROM identity_members m JOIN identity_users o ON o.id = m.user_id
	WHERE o.role = 'owner' ORDER BY m.org_id LIMIT 1 ON CONFLICT DO NOTHING`

// Invite makes the account now, so that roles can be set before they first sign in. They claim it with
// an invite link, or by signing in through an identity provider that vouches for the same email.
func (s *localStaff) Invite(ctx context.Context, email, given, family string) (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	tx, err := s.db.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `INSERT INTO identity_users (id, email, email_verified, name, role) VALUES ($1, $2, false, $3, 'member')`,
		id, strings.ToLower(email), strings.TrimSpace(given+" "+family)); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, joinOwnerOrg, id); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return localSubPrefix + id.String(), nil
}

func (s *localStaff) SetRoles(ctx context.Context, user string, _ staffGrant, roles []string) error {
	id, ok := localID(user)
	if !ok {
		return errors.New("staff: not an account on this panel")
	}
	if _, err := s.db.Pool.Exec(ctx, `INSERT INTO local_staff_roles (user_id, roles) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET roles = $2, updated_at = now()`, id, append([]string{}, roles...)); err != nil {
		return err
	}
	_, err := s.db.Pool.Exec(ctx, joinOwnerOrg, id)
	return err
}

// SetActive switches an account on or off. Off ends what it can do at the next request.
func (s *localStaff) SetActive(ctx context.Context, user string, active bool) error {
	id, ok := localID(user)
	if !ok {
		return errors.New("staff: not an account on this panel")
	}
	_, err := s.db.Pool.Exec(ctx, `UPDATE identity_users SET active = $2 WHERE id = $1 AND role <> 'owner'`, id, active)
	return err
}

// Claimed reports whether this person has signed in, so that an invite link would be no use.
func (s *localStaff) Claimed(ctx context.Context, sub string) (bool, error) {
	id, ok := localID(sub)
	if !ok {
		return false, nil
	}
	var claimed bool
	err := s.db.Pool.QueryRow(ctx, `SELECT email_verified FROM identity_users WHERE id = $1`, id).Scan(&claimed)
	return claimed, err
}

// Roles live only in the table the Team page edits, so there is nothing else to keep in step.
func (s *localStaff) RoleKeys(context.Context) ([]string, error)          { return nil, nil }
func (s *localStaff) PutRole(context.Context, string, string, bool) error { return nil }
func (s *localStaff) DeleteRole(context.Context, string) error            { return nil }
