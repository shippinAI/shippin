package identity

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/orgs"
)

var (
	ErrSetupUsed   = errors.New("setup code is not valid")
	ErrOwnerExists = errors.New("an owner already exists")
	ErrSignup      = errors.New("signup is closed")
	// ErrWaitlistClosed: the installation is not taking waitlist entries (sign-up mode is not waitlist).
	ErrWaitlistClosed = errors.New("the waitlist is not open")
	// ErrNoMail: the installation can't send email, so nothing that needs a confirmation can start.
	ErrNoMail     = errors.New("this installation can't send email yet, so the waitlist can't take new entries")
	ErrInvite     = errors.New("invite is not valid")
	ErrConflict   = errors.New("account is already linked")
	ErrNotFound   = errors.New("not found")
	ErrScope      = errors.New("missing scope")
	ErrSetupInput = errors.New("provide a valid email, username, owner name and internal organization name")
)

// Sender delivers one plain message. SMTP and the disposable file sink both implement it.
type Sender interface {
	Send(ctx context.Context, to, subject, body string) error
}

// Captcha checks a public form token. A none provider accepts every call.
type Captcha interface {
	OK(ctx context.Context, token, remote string) error
}

type NoneCaptcha struct{}

func (NoneCaptcha) OK(context.Context, string, string) error { return nil }

// Turnstile checks Cloudflare's siteverify endpoint.
type Turnstile struct {
	Secret string
	HTTP   *http.Client
}

func (t Turnstile) OK(ctx context.Context, token, remote string) error {
	if t.Secret == "" || token == "" {
		return errors.New("captcha is not configured")
	}
	form := url.Values{"secret": {t.Secret}, "response": {token}, "remoteip": {remote}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://challenges.cloudflare.com/turnstile/v0/siteverify", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := t.HTTP
	if client == nil {
		client = http.DefaultClient
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	var out struct {
		Success bool `json:"success"`
	}
	if json.NewDecoder(res.Body).Decode(&out) != nil || !out.Success {
		return errors.New("captcha was rejected")
	}
	return nil
}

// Service is the directory on the console database.
type Service struct {
	db      *pgxpool.Pool
	cfg     *config.Config
	send    Sender
	captcha Captcha
	limit   attemptLimiter
	// passkeyLimit is apart from limit, which allows far fewer, because signing in is ordinary.
	passkeyLimit  attemptLimiter
	passwordLimit attemptLimiter
	now           func() time.Time
	// signupMode is the live sign-up mode (the panel's setting); nil or empty uses the config file.
	signupMode func(context.Context) string
}

// CanMail reports whether this installation can send email.
func (s *Service) CanMail() bool { return s.send != nil }

// SetSignupMode makes sign-up follow a live value, such as the panel's setting.
func (s *Service) SetSignupMode(f func(context.Context) string) { s.signupMode = f }

// Signup is the sign-up mode in force now: open, invite_only or waitlist. Waitlist is invite-only
// for accounts, plus a public list people can join.
func (s *Service) Signup(ctx context.Context) string {
	if s.signupMode != nil {
		if m := s.signupMode(ctx); m != "" {
			return m
		}
	}
	return s.cfg.Directory.Signup
}

func New(pool *pgxpool.Pool, cfg *config.Config, send Sender, captcha Captcha) *Service {
	if captcha == nil {
		captcha = NoneCaptcha{}
	}
	return &Service{db: pool, cfg: cfg, send: send, captcha: captcha, limit: NewLimiter(5, 10*time.Minute), passkeyLimit: NewLimiter(30, time.Minute), passwordLimit: NewLimiter(8, 10*time.Minute), now: time.Now}
}

type User struct {
	ID            string
	Email         string
	Name          string
	Username      string
	Role          string
	EmailVerified bool
}

// SetupInput contains the complete first-run configuration. The setup code is
// the authorization to create the initial owner; all user and organization
// credentials are chosen by that owner.
// SetupInput creates the built-in setup account with the one-time setup code: its password, the
// internal organization and the installation brand. Nothing in it is about a person.
type SetupInput struct {
	Code, Password, Organization string
	Brand                        config.Brand
}

func (s *Service) audit(ctx context.Context, actor, action string, detail any) {
	raw, _ := json.Marshal(detail)
	_, _ = s.db.Exec(ctx, `INSERT INTO identity_audit (actor, action, detail) VALUES ($1,$2,$3)`, actor, action, raw)
}

// IssueSetup stores a new one-time owner code when no owner exists.
// HasOwner reports whether the first owner exists. An error reads as true so a database
// problem never advertises an open setup.
func (s *Service) HasOwner(ctx context.Context) bool {
	var n int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM identity_users WHERE role = 'owner'`).Scan(&n); err != nil {
		return true
	}
	return n > 0
}

func (s *Service) IssueSetup(ctx context.Context) (string, error) {
	var n int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM identity_users WHERE role = 'owner'`).Scan(&n); err != nil {
		return "", err
	}
	if n > 0 {
		return "", ErrOwnerExists
	}
	code, err := randomToken(18)
	if err != nil {
		return "", err
	}
	if _, err := s.db.Exec(ctx, `DELETE FROM identity_tokens WHERE kind = 'setup' AND used_at IS NULL`); err != nil {
		return "", err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO identity_tokens (id, kind, hash, expires_at) VALUES ($1,'setup',$2,$3)`,
		uuid.Must(uuid.NewV7()), hashToken(code), s.now().Add(24*time.Hour))
	return code, err
}

// RedeemSetup spends the one-time setup code on the built-in setup account (REL-03), the internal
// organization and the brand. The account owns the installation until a person takes it over; it has
// no recovery codes, so whoever ran setup keeps nothing once its password is changed.
func (s *Service) RedeemSetup(ctx context.Context, in SetupInput) (User, error) {
	in.Organization = strings.TrimSpace(in.Organization)
	if len(in.Organization) < 2 || len(in.Organization) > 120 {
		return User{}, ErrSetupInput
	}
	if err := validateBrand(in.Brand); err != nil {
		return User{}, err
	}
	passwordHash, err := hashPassword(in.Password)
	if err != nil {
		return User{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)
	// Serialize first-owner claims across concurrently issued setup codes.
	if _, err := tx.Exec(ctx, `LOCK TABLE identity_users IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return User{}, err
	}
	var ownerExists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity_users WHERE role='owner')`).Scan(&ownerExists); err != nil {
		return User{}, err
	}
	if ownerExists {
		return User{}, ErrOwnerExists
	}
	var id string
	err = tx.QueryRow(ctx, `UPDATE identity_tokens SET used_at = now() WHERE kind = 'setup' AND hash = $1 AND used_at IS NULL AND expires_at > now() RETURNING id`, hashToken(in.Code)).Scan(&id)
	if err != nil {
		return User{}, ErrSetupUsed
	}
	user, err := insertUser(ctx, tx, SetupEmail, "Setup account", "owner", true)
	if err != nil {
		return User{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE identity_users SET username=$2, password_hash=$3, builtin='setup' WHERE id=$1`, user.ID, SetupUsername, passwordHash); err != nil {
		return User{}, err
	}
	user.Username = SetupUsername
	org, err := orgs.ConfigureInternal(ctx, tx, in.Organization)
	if err != nil {
		return User{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO identity_members (org_id, user_id, role) VALUES ($1,$2,'owner')`, org, user.ID); err != nil {
		return User{}, err
	}
	brandJSON, _ := json.Marshal(in.Brand)
	if _, err := tx.Exec(ctx, `INSERT INTO identity_settings (key,value) VALUES ('branding',$1) ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value`, brandJSON); err != nil {
		return User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, err
	}
	s.audit(ctx, user.ID, "setup.account_created", map[string]string{"organization": in.Organization})
	s.emit(ctx, "user.signup", map[string]string{"user_id": user.ID, "via": "setup"})
	return user, nil
}

func insertUser(ctx context.Context, tx pgx.Tx, email, name, role string, verified bool) (User, error) {
	u := User{ID: uuid.Must(uuid.NewV7()).String(), Email: email, Name: name, Role: role, EmailVerified: verified}
	err := tx.QueryRow(ctx, `INSERT INTO identity_users (id, email, email_verified, name, role) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		u.ID, email, verified, name, role).Scan(&u.ID)
	return u, err
}

func insertRecovery(ctx context.Context, tx pgx.Tx, user string) ([]string, error) {
	out := make([]string, 0, 8)
	for i := 0; i < 8; i++ {
		code, err := randomToken(8)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO identity_recovery (id, user_id, code_hash) VALUES ($1,$2,$3)`, uuid.Must(uuid.NewV7()), user, hashToken(code)); err != nil {
			return nil, err
		}
		out = append(out, code)
	}
	return out, nil
}

// RequestMagic mails a sign-in link. New accounts follow directory.signup.
func (s *Service) RequestMagic(ctx context.Context, email, invite string) error {
	email = emailNorm(email)
	if s.cfg.Directory.UpstreamRequired {
		return ErrSignup
	}
	var exists bool
	_ = s.db.QueryRow(ctx, `SELECT true FROM identity_users WHERE email = $1`, email).Scan(&exists)
	if !exists {
		if s.Signup(ctx) != "open" && invite == "" {
			return ErrSignup
		}
	}
	raw, err := randomToken(24)
	if err != nil {
		return err
	}
	meta, _ := json.Marshal(map[string]string{"invite": invite})
	_, err = s.db.Exec(ctx, `INSERT INTO identity_tokens (id, kind, hash, email, meta, expires_at) VALUES ($1,'magic',$2,$3,$4,$5)`,
		uuid.Must(uuid.NewV7()), hashToken(raw), email, meta, s.now().Add(20*time.Minute))
	if err != nil {
		return err
	}
	if s.send == nil {
		return errors.New("mail is not configured")
	}
	return s.send.Send(ctx, email, "Sign in", "Open this link to sign in:\n"+raw+"\n")
}

// RedeemMagic consumes a magic link and returns the user.
func (s *Service) RedeemMagic(ctx context.Context, raw string) (User, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)
	var email string
	var meta []byte
	err = tx.QueryRow(ctx, `UPDATE identity_tokens SET used_at = now() WHERE kind = 'magic' AND hash = $1 AND used_at IS NULL AND expires_at > now() RETURNING email, meta`, hashToken(raw)).Scan(&email, &meta)
	if err != nil {
		return User{}, ErrNotFound
	}
	var u User
	err = tx.QueryRow(ctx, `SELECT id, email, name, role, email_verified FROM identity_users WHERE email = $1`, email).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.EmailVerified)
	if errors.Is(err, pgx.ErrNoRows) {
		var extra struct {
			Invite string `json:"invite"`
		}
		_ = json.Unmarshal(meta, &extra)
		if s.Signup(ctx) != "open" {
			if err := consumeInvite(ctx, tx, extra.Invite, email); err != nil {
				return User{}, err
			}
		}
		u, err = insertUser(ctx, tx, email, "", "member", true)
		if err != nil {
			return User{}, err
		}
		s.emit(ctx, "user.signup", map[string]string{"user_id": u.ID, "via": "magic"})
	} else if err != nil {
		return User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, err
	}
	return u, nil
}

func consumeInvite(ctx context.Context, tx pgx.Tx, raw, email string) error {
	if raw == "" {
		return ErrInvite
	}
	var id string
	err := tx.QueryRow(ctx, `UPDATE identity_invites SET uses = uses + 1 WHERE hash = $1 AND uses < max_uses AND expires_at > now() AND (email = '' OR email = $2) RETURNING id`, hashToken(raw), email).Scan(&id)
	if err != nil {
		return ErrInvite
	}
	return nil
}

// LinkUpstream attaches an issuer subject to a local user.
func (s *Service) LinkUpstream(ctx context.Context, issuer, sub, email string, verified bool) (User, error) {
	issuer = strings.TrimRight(strings.TrimSpace(issuer), "/")
	email = emailNorm(email)
	var u User
	err := s.db.QueryRow(ctx, `SELECT u.id, u.email, u.name, u.role, u.email_verified FROM identity_links l JOIN identity_users u ON u.id = l.user_id WHERE l.issuer = $1 AND l.sub = $2`, issuer, sub).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.EmailVerified)
	if err == nil {
		return u, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return User{}, err
	}
	if verified && email != "" {
		err = s.db.QueryRow(ctx, `SELECT id, email, name, role, email_verified FROM identity_users WHERE email = $1`, email).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.EmailVerified)
		if err == nil {
			var other string
			err = s.db.QueryRow(ctx, `SELECT sub FROM identity_links WHERE user_id = $1 AND issuer = $2`, u.ID, issuer).Scan(&other)
			if err == nil && other != sub {
				return User{}, ErrConflict
			}
			if _, err := s.db.Exec(ctx, `INSERT INTO identity_links (issuer, sub, user_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, issuer, sub, u.ID); err != nil {
				return User{}, err
			}
			// The provider vouched for this email, which is what an invited account was waiting for.
			if _, err := s.db.Exec(ctx, `UPDATE identity_users SET email_verified = true WHERE id = $1`, u.ID); err != nil {
				return User{}, err
			}
			s.audit(ctx, u.ID, "account.link", map[string]string{"issuer": issuer})
			return u, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return User{}, err
		}
	}
	if s.Signup(ctx) != "open" {
		return User{}, ErrSignup
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)
	u, err = insertUser(ctx, tx, email, "", "member", verified)
	if err != nil {
		return User{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO identity_links (issuer, sub, user_id) VALUES ($1,$2,$3)`, issuer, sub, u.ID); err != nil {
		return User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, err
	}
	s.emit(ctx, "user.signup", map[string]string{"user_id": u.ID, "via": "upstream"})
	return u, nil
}

// CreateInvite stores an invite and returns the raw code once.
func (s *Service) CreateInvite(ctx context.Context, actor, email, role string, uses int, expires time.Time) (string, error) {
	if uses < 1 {
		uses = 1
	}
	if role == "" {
		role = "member"
	}
	if _, err := uuid.Parse(actor); err != nil {
		actor = ""
	}
	raw, err := randomToken(16)
	if err != nil {
		return "", err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO identity_invites (id, hash, email, role, max_uses, expires_at, created_by) VALUES ($1,$2,$3,$4,$5,$6,NULLIF($7,'')::uuid)`,
		uuid.Must(uuid.NewV7()), hashToken(raw), emailNorm(email), role, uses, expires, actor)
	if err != nil {
		return "", err
	}
	s.audit(ctx, actor, "invite.create", map[string]string{"email": emailNorm(email)})
	return raw, nil
}

// RedeemInvite makes a member from an invite code, or hands them the account someone made for them. An
// account that has already been signed in to is never taken over this way. Recovery codes come back
// once, because with no mail or passkey they are how this person signs in next time.
func (s *Service) RedeemInvite(ctx context.Context, raw, email, name string) (User, []string, error) {
	email = emailNorm(email)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return User{}, nil, err
	}
	defer tx.Rollback(ctx)
	if err := consumeInvite(ctx, tx, raw, email); err != nil {
		return User{}, nil, err
	}
	var u User
	err = tx.QueryRow(ctx, `SELECT id, email, name, role, email_verified FROM identity_users WHERE email = $1`, email).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.EmailVerified)
	switch {
	case err == nil && u.EmailVerified:
		return User{}, nil, ErrInvite
	case err == nil:
		if _, err := tx.Exec(ctx, `UPDATE identity_users SET email_verified = true, name = CASE WHEN $2 = '' THEN name ELSE $2 END WHERE id = $1`, u.ID, name); err != nil {
			return User{}, nil, err
		}
		u.EmailVerified = true
	case errors.Is(err, pgx.ErrNoRows):
		if u, err = insertUser(ctx, tx, email, name, "member", true); err != nil {
			return User{}, nil, err
		}
	default:
		return User{}, nil, err
	}
	codes, err := insertRecovery(ctx, tx, u.ID)
	if err != nil {
		return User{}, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, nil, err
	}
	s.audit(ctx, u.ID, "invite.redeemed", map[string]string{"email": email})
	s.emit(ctx, "invite.redeemed", map[string]string{"user_id": u.ID})
	return u, codes, nil
}

// JoinWaitlist records a pending row and mails a confirmation token.
func (s *Service) JoinWaitlist(ctx context.Context, email, name, company, notes, captcha, remote string) error {
	if s.Signup(ctx) != "waitlist" {
		return ErrWaitlistClosed
	}
	if s.send == nil {
		return ErrNoMail // checked first: an entry nobody can confirm would wait forever
	}
	if !s.limit.Allow(remote, s.now()) {
		return errors.New("rate limit")
	}
	if err := s.captcha.OK(ctx, captcha, remote); err != nil {
		return err
	}
	email = emailNorm(email)
	raw, err := randomToken(18)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO identity_waitlist (id, email, name, company, notes, status) VALUES ($1,$2,$3,$4,$5,'pending')
		ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name, company = EXCLUDED.company, notes = EXCLUDED.notes WHERE identity_waitlist.status = 'pending'`,
		uuid.Must(uuid.NewV7()), email, name, company, notes)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO identity_tokens (id, kind, hash, email, expires_at) VALUES ($1,'waitlist',$2,$3,$4)`,
		uuid.Must(uuid.NewV7()), hashToken(raw), email, s.now().Add(48*time.Hour))
	if err != nil {
		return err
	}
	if s.send == nil {
		return errors.New("mail is not configured")
	}
	body := "Confirm your request to join the waitlist.\n"
	if o := strings.TrimRight(s.cfg.Directory.Origin, "/"); o != "" {
		body += "Open this link:\n" + o + "/waitlist?token=" + raw + "\n\nOr enter this code on the waitlist page:\n"
	}
	return s.send.Send(ctx, email, "Confirm your request", body+raw+"\n")
}

// ConfirmWaitlist moves a pending row to confirmed.
func (s *Service) ConfirmWaitlist(ctx context.Context, raw string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var email string
	err = tx.QueryRow(ctx, `UPDATE identity_tokens SET used_at = now() WHERE kind = 'waitlist' AND hash = $1 AND used_at IS NULL AND expires_at > now() RETURNING email`, hashToken(raw)).Scan(&email)
	if err != nil {
		return ErrNotFound
	}
	tag, err := tx.Exec(ctx, `UPDATE identity_waitlist SET status = 'confirmed' WHERE email = $1 AND status = 'pending'`, email)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.audit(ctx, email, "waitlist.confirmed", map[string]string{"email": email})
	s.emit(ctx, "waitlist.joined", map[string]string{"email": email})
	return nil
}

type WaitRow struct {
	ID, Email, Name, Company, Notes, Status string
	CreatedAt                               time.Time
}

func (s *Service) SearchWaitlist(ctx context.Context, q string) ([]WaitRow, error) {
	q = "%" + strings.TrimSpace(q) + "%"
	rows, err := s.db.Query(ctx, `SELECT id::text, email, name, company, notes, status, created_at FROM identity_waitlist WHERE email ILIKE $1 OR name ILIKE $1 ORDER BY created_at`, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WaitRow
	for rows.Next() {
		var w WaitRow
		if err := rows.Scan(&w.ID, &w.Email, &w.Name, &w.Company, &w.Notes, &w.Status, &w.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Service) ExportWaitlist(ctx context.Context) (string, error) {
	rows, err := s.SearchWaitlist(ctx, "")
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("email,name,company,notes,status\n")
	for _, w := range rows {
		b.WriteString(csv(w.Email) + "," + csv(w.Name) + "," + csv(w.Company) + "," + csv(w.Notes) + "," + csv(w.Status) + "\n")
	}
	return b.String(), nil
}

func csv(s string) string {
	if strings.ContainsAny(s, ",\"\n") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

// ErrNotConfirmed: only waitlist rows whose address was confirmed can be invited.
var ErrNotConfirmed = errors.New("only confirmed rows can be invited")

// BulkInvite invites confirmed waitlist rows and returns the raw codes.
func (s *Service) BulkInvite(ctx context.Context, actor string, ids []string) ([]string, error) {
	var codes []string
	for _, id := range ids {
		_, code, err := s.InviteFromWaitlist(ctx, actor, id)
		if err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	return codes, nil
}

// InviteFromWaitlist invites one confirmed waitlist row for 14 days and returns its address and raw code.
func (s *Service) InviteFromWaitlist(ctx context.Context, actor, id string) (string, string, error) {
	var email, status string
	if err := s.db.QueryRow(ctx, `SELECT email, status FROM identity_waitlist WHERE id::text = $1`, id).Scan(&email, &status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", ErrNotFound
		}
		return "", "", err
	}
	if status != "confirmed" {
		return "", "", ErrNotConfirmed
	}
	code, err := s.CreateInvite(ctx, actor, email, "member", 1, s.now().Add(14*24*time.Hour))
	if err != nil {
		return "", "", err
	}
	if _, err := s.db.Exec(ctx, `UPDATE identity_waitlist SET status = 'invited' WHERE id::text = $1`, id); err != nil {
		return "", "", err
	}
	return email, code, nil
}

// SendInvite mails an invitation link. It fails when the installation has no mail.
func (s *Service) SendInvite(ctx context.Context, email, link string) error {
	if s.send == nil {
		return errors.New("mail is not configured")
	}
	return s.send.Send(ctx, email, "You're invited", "You have been invited. Open this link to create your account:\n"+link+"\n")
}

// CreateAPIKey returns the raw key once.
func (s *Service) CreateAPIKey(ctx context.Context, name string, scopes []string) (string, error) {
	raw, err := randomToken(24)
	if err != nil {
		return "", err
	}
	key := "shp_" + raw
	_, err = s.db.Exec(ctx, `INSERT INTO identity_api_keys (id, name, hash, prefix, scopes) VALUES ($1,$2,$3,$4,$5)`,
		uuid.Must(uuid.NewV7()), name, hashToken(key), key[:8], scopes)
	return key, err
}

// KeyScopes returns scopes for a raw API key.
func (s *Service) KeyScopes(ctx context.Context, raw string) ([]string, error) {
	var scopes []string
	err := s.db.QueryRow(ctx, `SELECT scopes FROM identity_api_keys WHERE hash = $1`, hashToken(raw)).Scan(&scopes)
	if err != nil {
		return nil, ErrNotFound
	}
	return scopes, nil
}

func HasScope(scopes []string, want string) bool {
	for _, s := range scopes {
		if s == want {
			return true
		}
	}
	return false
}

// SaveBranding stores the admin overlay.
func (s *Service) SaveBranding(ctx context.Context, brand config.Brand) error {
	if err := validateBrand(brand); err != nil {
		return err
	}
	raw, _ := json.Marshal(brand)
	_, err := s.db.Exec(ctx, `INSERT INTO identity_settings (key, value) VALUES ('branding', $1) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`, raw)
	return err
}

// Branding returns the overlay when one has been saved.
func (s *Service) Branding(ctx context.Context) (config.Brand, bool) {
	var raw []byte
	if err := s.db.QueryRow(ctx, `SELECT value FROM identity_settings WHERE key = 'branding'`).Scan(&raw); err != nil {
		return config.Brand{}, false
	}
	var b config.Brand
	if json.Unmarshal(raw, &b) != nil {
		return config.Brand{}, false
	}
	return b, true
}

// ConfigVersion is the number of branding writes plus one.
func (s *Service) ConfigVersion(ctx context.Context) int {
	var n int
	_ = s.db.QueryRow(ctx, `SELECT count(*) FROM identity_audit WHERE action = 'config.write'`).Scan(&n)
	return n + 1
}

func (s *Service) NoteConfigWrite(ctx context.Context, actor string) {
	s.audit(ctx, actor, "config.write", map[string]string{})
}

// AddWebhook stores a subscription and returns the signing secret once.
func (s *Service) AddWebhook(ctx context.Context, hookURL string, events []string) (string, error) {
	secret, err := randomToken(24)
	if err != nil {
		return "", err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO identity_webhooks (id, url, secret, events) VALUES ($1,$2,$3,$4)`,
		uuid.Must(uuid.NewV7()), hookURL, []byte(secret), events)
	return secret, err
}

func (s *Service) emit(ctx context.Context, event string, payload any) {
	body, _ := json.Marshal(map[string]any{"event": event, "data": payload})
	rows, err := s.db.Query(ctx, `SELECT url, secret FROM identity_webhooks WHERE $1 = ANY (events)`, event)
	if err != nil {
		return
	}
	defer rows.Close()
	now := s.now()
	for rows.Next() {
		var hookURL string
		var secret []byte
		if rows.Scan(&hookURL, &secret) != nil {
			continue
		}
		sig := SignWebhook(secret, body, now)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, hookURL, bytes.NewReader(body))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Shippin-Signature", sig)
		res, err := http.DefaultClient.Do(req)
		if err == nil {
			io.Copy(io.Discard, res.Body)
			res.Body.Close()
		}
	}
}

// EmitHostEnrolled tells subscribers a host finished enrollment. The credential is not included.
func (s *Service) EmitHostEnrolled(ctx context.Context, serverID, hostname string) {
	s.audit(ctx, serverID, "host.enrolled", map[string]string{"hostname": hostname})
	s.emit(ctx, "host.enrolled", map[string]string{"server_id": serverID, "hostname": hostname})
}

// AuthorizeCode stores an authorization code bound to a PKCE challenge.
func (s *Service) AuthorizeCode(ctx context.Context, clientID, userID, redirect, challenge string) (string, error) {
	if challenge == "" {
		return "", errors.New("pkce challenge is required")
	}
	ok := false
	for _, r := range s.cfg.Directory.Redirects {
		if r == redirect {
			ok = true
		}
	}
	if clientID != s.cfg.Directory.ClientID || !ok {
		return "", errors.New("client or redirect is not registered")
	}
	raw, err := randomToken(24)
	if err != nil {
		return "", err
	}
	_, err = s.db.Exec(ctx, `INSERT INTO identity_oauth_codes (hash, client_id, user_id, challenge, redirect, expires_at) VALUES ($1,$2,$3,$4,$5,$6)`,
		hashToken(raw), clientID, userID, challenge, redirect, s.now().Add(2*time.Minute))
	return raw, err
}

// ExchangeCode returns a refresh token and the user for a PKCE code.
func (s *Service) ExchangeCode(ctx context.Context, code, verifier, clientID, redirect string) (string, User, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", User{}, err
	}
	defer tx.Rollback(ctx)
	var challenge, userID, gotRedirect, gotClient string
	err = tx.QueryRow(ctx, `UPDATE identity_oauth_codes SET used_at = now() WHERE hash = $1 AND used_at IS NULL AND expires_at > now() RETURNING challenge, user_id::text, redirect, client_id`, hashToken(code)).Scan(&challenge, &userID, &gotRedirect, &gotClient)
	if err != nil || gotClient != clientID || gotRedirect != redirect || !PKCEMatch(verifier, challenge) {
		return "", User{}, errors.New("authorization code was rejected")
	}
	refresh, err := randomToken(24)
	if err != nil {
		return "", User{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO identity_oauth_refresh (hash, family, client_id, user_id, expires_at) VALUES ($1,$2,$3,$4,$5)`,
		hashToken(refresh), uuid.Must(uuid.NewV7()), clientID, userID, s.now().Add(14*24*time.Hour)); err != nil {
		return "", User{}, err
	}
	var u User
	if err := tx.QueryRow(ctx, `SELECT id::text, email, name, role, email_verified FROM identity_users WHERE id = $1`, userID).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.EmailVerified); err != nil {
		return "", User{}, err
	}
	return refresh, u, tx.Commit(ctx)
}

// RotateRefresh replaces a refresh token. Reuse after rotation revokes the family.
func (s *Service) RotateRefresh(ctx context.Context, raw, clientID string) (string, User, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", User{}, err
	}
	defer tx.Rollback(ctx)
	var family, userID string
	var replaced, revoked bool
	err = tx.QueryRow(ctx, `SELECT family::text, user_id::text, replaced, revoked FROM identity_oauth_refresh WHERE hash = $1 AND client_id = $2 AND expires_at > now()`, hashToken(raw), clientID).Scan(&family, &userID, &replaced, &revoked)
	if err != nil {
		return "", User{}, errors.New("refresh token was rejected")
	}
	if revoked || replaced {
		_, _ = tx.Exec(ctx, `UPDATE identity_oauth_refresh SET revoked = true WHERE family = $1`, family)
		_ = tx.Commit(ctx)
		return "", User{}, errors.New("refresh token was reused")
	}
	next, err := randomToken(24)
	if err != nil {
		return "", User{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE identity_oauth_refresh SET replaced = true WHERE hash = $1`, hashToken(raw)); err != nil {
		return "", User{}, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO identity_oauth_refresh (hash, family, client_id, user_id, expires_at) VALUES ($1,$2,$3,$4,$5)`,
		hashToken(next), family, clientID, userID, s.now().Add(14*24*time.Hour)); err != nil {
		return "", User{}, err
	}
	var u User
	if err := tx.QueryRow(ctx, `SELECT id::text, email, name, role, email_verified FROM identity_users WHERE id = $1`, userID).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.EmailVerified); err != nil {
		return "", User{}, err
	}
	return next, u, tx.Commit(ctx)
}

// RedeemRecovery consumes one recovery code.
func (s *Service) RedeemRecovery(ctx context.Context, email, code string) (User, error) {
	email = emailNorm(email)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)
	var u User
	err = tx.QueryRow(ctx, `SELECT id::text, email, name, role, email_verified FROM identity_users WHERE email = $1`, email).Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.EmailVerified)
	if err != nil {
		return User{}, ErrNotFound
	}
	tag, err := tx.Exec(ctx, `UPDATE identity_recovery SET used_at = now() WHERE user_id = $1 AND code_hash = $2 AND used_at IS NULL`, u.ID, hashToken(code))
	if err != nil || tag.RowsAffected() != 1 {
		return User{}, ErrNotFound
	}
	return u, tx.Commit(ctx)
}

// passkeyChallengeFor is how long someone has to answer a challenge.
const passkeyChallengeFor = 5 * time.Minute

// ErrChallenge is a challenge this panel did not issue, or that has been used or has expired.
var ErrChallenge = errors.New("passkey challenge is not valid")

// ErrRateLimited is too many attempts from one address.
var ErrRateLimited = errors.New("too many attempts")

// NewPasskeyChallenge issues a challenge for one passkey ceremony.
func (s *Service) NewPasskeyChallenge(ctx context.Context, from string) ([]byte, error) {
	if !s.passkeyLimit.Allow(from, s.now()) {
		return nil, ErrRateLimited
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	if _, err := s.db.Exec(ctx, `DELETE FROM identity_passkey_challenges WHERE expires_at < now()`); err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO identity_passkey_challenges (hash, expires_at) VALUES ($1, $2)`, sum[:], s.now().Add(passkeyChallengeFor)); err != nil {
		return nil, err
	}
	return raw, nil
}

// UsePasskeyChallenge spends a challenge. Whatever the ceremony's outcome, it cannot be answered twice.
func (s *Service) UsePasskeyChallenge(ctx context.Context, raw []byte) error {
	sum := sha256.Sum256(raw)
	tag, err := s.db.Exec(ctx, `DELETE FROM identity_passkey_challenges WHERE hash = $1 AND expires_at >= now()`, sum[:])
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrChallenge
	}
	return nil
}

// PasskeyRow is one credential as its owner sees it.
type PasskeyRow struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

// ListPasskeys is one person's credentials.
func (s *Service) ListPasskeys(ctx context.Context, user string) ([]PasskeyRow, error) {
	rows, err := s.db.Query(ctx, `SELECT id::text, created_at FROM identity_passkeys WHERE user_id = $1 ORDER BY created_at`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PasskeyRow{}
	for rows.Next() {
		var p PasskeyRow
		if err := rows.Scan(&p.ID, &p.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeletePasskey removes one of this person's own credentials.
func (s *Service) DeletePasskey(ctx context.Context, user, id string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM identity_passkeys WHERE id = $1 AND user_id = $2`, id, user)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

// SavePasskey stores a credential for a user.
func (s *Service) SavePasskey(ctx context.Context, user string, key Passkey) error {
	_, err := s.db.Exec(ctx, `INSERT INTO identity_passkeys (id, user_id, credential_id, public_key, sign_count) VALUES ($1,$2,$3,$4,$5)`,
		uuid.Must(uuid.NewV7()), user, key.CredentialID, key.PublicKey, key.SignCount)
	return err
}

// Passkeys lists credentials for an email.
func (s *Service) Passkeys(ctx context.Context, email string) (string, []Passkey, error) {
	var id string
	if err := s.db.QueryRow(ctx, `SELECT id::text FROM identity_users WHERE email = $1`, emailNorm(email)).Scan(&id); err != nil {
		return "", nil, ErrNotFound
	}
	rows, err := s.db.Query(ctx, `SELECT credential_id, public_key, sign_count FROM identity_passkeys WHERE user_id = $1`, id)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	var out []Passkey
	for rows.Next() {
		var k Passkey
		var count int64
		if err := rows.Scan(&k.CredentialID, &k.PublicKey, &count); err != nil {
			return "", nil, err
		}
		k.SignCount = uint32(count)
		out = append(out, k)
	}
	return id, out, rows.Err()
}

// BumpPasskey updates the counter after a successful assertion.
func (s *Service) BumpPasskey(ctx context.Context, cred []byte, count uint32) error {
	_, err := s.db.Exec(ctx, `UPDATE identity_passkeys SET sign_count = $2 WHERE credential_id = $1`, cred, count)
	return err
}

// FileSender appends messages to a path. It is the disposable mail provider.
type FileSender struct{ Path string }

func (f FileSender) Send(_ context.Context, to, subject, body string) error {
	fh, err := os.OpenFile(f.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer fh.Close()
	enc := json.NewEncoder(fh)
	return enc.Encode(map[string]string{"to": to, "subject": subject, "body": body})
}
