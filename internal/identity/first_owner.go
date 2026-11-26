package identity

import (
	"context"
	"encoding/json"
	"net/mail"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"shippin.cloud/console/internal/platform/config"
	"shippin.cloud/console/internal/tenancy/orgs"
)

// The installation's first owner (REL-03). There is no setup code. While no owner exists, the first
// person to open the panel creates the owner account, an accepted risk the owner chose for a simple
// install (2026-11-25); or the installation names its owner up front (SHIPPIN_OWNER_EMAIL, internal/app)
// and nobody has to visit the panel at all. Either way the first claim wins and every later one is
// refused.

// DefaultOrganization names the installation's organization when the claim does not.
const DefaultOrganization = "Operations"

// FirstOwnerInput is the person claiming the installation, and what they call it.
type FirstOwnerInput struct {
	Name, Email, Username, Password, Organization string
	// Brand is the installation's branding; with no name, the panel keeps its default.
	Brand config.Brand
}

// Claim is how the installation was claimed, for the audit log: Via is "browser" or "environment",
// From the client address of a browser claim.
type Claim struct {
	Via, From string
	// Recovery issues the owner's recovery codes, shown once. A claim nobody watches skips them; the
	// owner makes some later under Account > Security.
	Recovery bool
}

var usernameChars = regexp.MustCompile(`[^a-z0-9._-]+`)

// usernameFor is a username made from an email address's local part, or "owner".
func usernameFor(email string) string {
	local, _, _ := strings.Cut(email, "@")
	u := strings.Trim(usernameChars.ReplaceAllString(strings.ToLower(local), "-"), "-._")
	if len(u) > 32 {
		u = u[:32]
	}
	if !validUsername(u) {
		return "owner"
	}
	return u
}

// ClaimInstallation creates the first owner, the installation's organization and its brand in one
// transaction, and signs nothing in. It returns ErrOwnerExists once anyone owns the installation, so
// two people claiming at once cannot both win.
func (s *Service) ClaimInstallation(ctx context.Context, in FirstOwnerInput, c Claim) (User, []string, error) {
	in.Email = emailNorm(in.Email)
	in.Name = strings.TrimSpace(in.Name)
	in.Username = strings.ToLower(strings.TrimSpace(in.Username))
	in.Organization = strings.TrimSpace(in.Organization)
	if in.Organization == "" {
		in.Organization = DefaultOrganization
	}
	address, emailErr := mail.ParseAddress(in.Email)
	if emailErr != nil || address.Address != in.Email || len(in.Name) < 1 || len(in.Name) > 120 ||
		len(in.Organization) < 2 || len(in.Organization) > 120 || (in.Username != "" && !validUsername(in.Username)) {
		return User{}, nil, ErrSetupInput
	}
	if in.Brand.Name != "" {
		if err := validateBrand(in.Brand); err != nil {
			return User{}, nil, err
		}
	}
	hash, err := hashPassword(in.Password)
	if err != nil {
		return User{}, nil, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return User{}, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }() // a no-op once committed
	// Concurrent claims queue here; whoever is second finds the owner.
	if _, err := tx.Exec(ctx, `LOCK TABLE identity_users IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return User{}, nil, err
	}
	var ownerExists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity_users WHERE role='owner')`).Scan(&ownerExists); err != nil {
		return User{}, nil, err
	}
	if ownerExists {
		return User{}, nil, ErrOwnerExists
	}
	var taken bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity_users WHERE email=$1)`, in.Email).Scan(&taken); err != nil {
		return User{}, nil, err
	}
	if taken {
		return User{}, nil, ErrConflict
	}
	username, err := freeUsername(ctx, tx, in.Username, in.Email)
	if err != nil {
		return User{}, nil, err
	}
	user, err := insertUser(ctx, tx, in.Email, in.Name, "owner", true)
	if err != nil {
		return User{}, nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE identity_users SET username=$2, password_hash=$3 WHERE id=$1`, user.ID, username, hash); err != nil {
		return User{}, nil, err
	}
	user.Username = username
	org, err := orgs.ConfigureInternal(ctx, tx, in.Organization)
	if err != nil {
		return User{}, nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO identity_members (org_id, user_id, role) VALUES ($1,$2,'owner')`, org, user.ID); err != nil {
		return User{}, nil, err
	}
	if in.Brand.Name != "" {
		brandJSON, _ := json.Marshal(in.Brand)
		if _, err := tx.Exec(ctx, `INSERT INTO identity_settings (key,value) VALUES ('branding',$1) ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value`, brandJSON); err != nil {
			return User{}, nil, err
		}
	}
	var codes []string
	if c.Recovery {
		if codes, err = insertRecovery(ctx, tx, user.ID); err != nil {
			return User{}, nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, nil, err
	}
	s.audit(ctx, user.ID, "setup.owner_claimed", map[string]string{"via": c.Via, "from": c.From, "organization": in.Organization, "username": username})
	s.emit(ctx, "user.signup", map[string]string{"user_id": user.ID, "via": "setup_" + c.Via})
	return user, codes, nil
}

// freeUsername is the asked-for username, or one made from the email address, with a number added when
// an existing account (such as a development sign-in) already has it.
func freeUsername(ctx context.Context, tx pgx.Tx, asked, email string) (string, error) {
	base := asked
	if base == "" {
		base = usernameFor(email)
	}
	for i := 1; i <= 20; i++ {
		u := base
		if i > 1 {
			suffix := strconv.Itoa(i)
			if len(u)+len(suffix) > 32 {
				u = u[:32-len(suffix)]
			}
			u += suffix
		}
		var taken bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity_users WHERE lower(username)=$1)`, u).Scan(&taken); err != nil {
			return "", err
		}
		if !taken {
			return u, nil
		}
		if asked != "" {
			return "", ErrConflict
		}
	}
	return "", ErrConflict
}
