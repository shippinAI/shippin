package identity

import (
	"context"
	"errors"
	"net/mail"
	"strings"

	"github.com/jackc/pgx/v5"
)

// The built-in setup account (REL-03). /setup creates it with the one-time setup code, so whoever
// installs Shippin, a person or an AI assistant, can finish setting it up. A person then takes it over
// in two steps the panel keeps in front of them: set its password (ending every other session of it)
// and create their own owner account.

const (
	// SetupUsername is how the setup account signs in.
	SetupUsername = "setup"
	// SetupEmail is the setup account's address. It is in the reserved .invalid domain, so nothing is
	// ever mailed to it and no person can register it.
	SetupEmail = "setup@setup.invalid"
)

// ErrNotOwner: only the setup account or an installation owner can take setup over.
var ErrNotOwner = errors.New("only the setup account or an installation owner can do this")

// Handover is where taking over the setup account stands.
type Handover struct {
	// Exists: this installation was set up with the setup account.
	Exists bool `json:"exists"`
	// SetupID is the setup account's user id.
	SetupID string `json:"-"`
	// PasswordSet: a person has set the setup account's password since setup.
	PasswordSet bool `json:"password_set"`
	// Owners are the owner accounts people created, by username.
	Owners []string `json:"owners"`
}

// Pending: the panel still has a takeover step to show.
func (h Handover) Pending() bool { return h.Exists && (!h.PasswordSet || len(h.Owners) == 0) }

// SetupHandover reads the takeover state.
func (s *Service) SetupHandover(ctx context.Context) (Handover, error) {
	var h Handover
	var takenOver *string
	err := s.db.QueryRow(ctx, `SELECT id::text, taken_over_at::text FROM identity_users WHERE builtin='setup'`).Scan(&h.SetupID, &takenOver)
	if errors.Is(err, pgx.ErrNoRows) {
		return Handover{Owners: []string{}}, nil
	}
	if err != nil {
		return Handover{}, err
	}
	h.Exists, h.PasswordSet = true, takenOver != nil
	rows, err := s.db.Query(ctx, `SELECT COALESCE(NULLIF(username,''), email) FROM identity_users
		WHERE role='owner' AND active AND builtin IS NULL ORDER BY created_at`)
	if err != nil {
		return Handover{}, err
	}
	defer rows.Close()
	h.Owners = []string{}
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			return Handover{}, err
		}
		h.Owners = append(h.Owners, u)
	}
	return h, rows.Err()
}

// mayTakeOver: the actor is the setup account or an active owner.
func mayTakeOver(ctx context.Context, q pgx.Tx, actor string) error {
	var ok bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity_users WHERE id::text=$1 AND active AND role='owner')`, actor).Scan(&ok)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotOwner
	}
	return nil
}

// TakeOverSetup sets the setup account's password and removes every other way in that whoever ran
// setup could hold: passkeys and recovery codes. The caller ends its other sessions. It returns the
// setup account's id.
func (s *Service) TakeOverSetup(ctx context.Context, actor, password string) (string, error) {
	hash, err := hashPassword(password)
	if err != nil {
		return "", err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)
	if err := mayTakeOver(ctx, tx, actor); err != nil {
		return "", err
	}
	var id string
	if err := tx.QueryRow(ctx, `UPDATE identity_users SET password_hash=$1, taken_over_at=now() WHERE builtin='setup' RETURNING id::text`, hash).Scan(&id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	for _, q := range []string{`DELETE FROM identity_passkeys WHERE user_id::text=$1`, `DELETE FROM identity_recovery WHERE user_id::text=$1`,
		`DELETE FROM access_keys WHERE user_id::text=$1`} {
		if _, err := tx.Exec(ctx, q, id); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	s.audit(ctx, actor, "setup.taken_over", map[string]string{"setup_account": id})
	return id, nil
}

// OwnerInput is a person's own owner account.
type OwnerInput struct {
	Username, Email, Name, Password string
}

// CreateOwner makes a person's own owner account, in the internal organization, and returns its
// recovery codes once.
func (s *Service) CreateOwner(ctx context.Context, actor string, in OwnerInput) (User, []string, error) {
	in.Email = emailNorm(in.Email)
	in.Username = strings.ToLower(strings.TrimSpace(in.Username))
	in.Name = strings.TrimSpace(in.Name)
	address, emailErr := mail.ParseAddress(in.Email)
	if emailErr != nil || address.Address != in.Email || !validUsername(in.Username) || in.Username == SetupUsername || len(in.Name) < 1 || len(in.Name) > 120 {
		return User{}, nil, ErrSetupInput
	}
	hash, err := hashPassword(in.Password)
	if err != nil {
		return User{}, nil, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return User{}, nil, err
	}
	defer tx.Rollback(ctx)
	if err := mayTakeOver(ctx, tx, actor); err != nil {
		return User{}, nil, err
	}
	var taken bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity_users WHERE email=$1 OR lower(username)=$2)`, in.Email, in.Username).Scan(&taken); err != nil {
		return User{}, nil, err
	}
	if taken {
		return User{}, nil, ErrConflict
	}
	// The internal organization is the one the setup account, or the acting owner, owns.
	var org string
	err = tx.QueryRow(ctx, `SELECT m.org_id::text FROM identity_members m JOIN identity_users u ON u.id = m.user_id
		WHERE m.role='owner' AND (u.builtin='setup' OR u.id::text=$1) ORDER BY u.builtin NULLS LAST LIMIT 1`, actor).Scan(&org)
	if err != nil {
		return User{}, nil, err
	}
	u, err := insertUser(ctx, tx, in.Email, in.Name, "owner", true)
	if err != nil {
		return User{}, nil, err
	}
	if _, err := tx.Exec(ctx, `UPDATE identity_users SET username=$2, password_hash=$3 WHERE id=$1`, u.ID, in.Username, hash); err != nil {
		return User{}, nil, err
	}
	u.Username = in.Username
	if _, err := tx.Exec(ctx, `INSERT INTO identity_members (org_id, user_id, role) VALUES ($1,$2,'owner')`, org, u.ID); err != nil {
		return User{}, nil, err
	}
	codes, err := insertRecovery(ctx, tx, u.ID)
	if err != nil {
		return User{}, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, nil, err
	}
	s.audit(ctx, actor, "setup.owner_created", map[string]string{"user_id": u.ID, "username": u.Username})
	s.emit(ctx, "user.signup", map[string]string{"user_id": u.ID, "via": "setup_handover"})
	return u, codes, nil
}
