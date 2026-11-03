package identity

import (
	"bytes"
	"context"
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
)

var (
	ErrSetupUsed   = errors.New("setup code is not valid")
	ErrOwnerExists = errors.New("an owner already exists")
	ErrSignup      = errors.New("signup is closed")
	ErrInvite      = errors.New("invite is not valid")
	ErrConflict    = errors.New("account is already linked")
	ErrNotFound    = errors.New("not found")
	ErrScope       = errors.New("missing scope")
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
	limit   *Limiter
	now     func() time.Time
}

func New(pool *pgxpool.Pool, cfg *config.Config, send Sender, captcha Captcha) *Service {
	if captcha == nil {
		captcha = NoneCaptcha{}
	}
	return &Service{db: pool, cfg: cfg, send: send, captcha: captcha, limit: NewLimiter(5, 10*time.Minute), now: time.Now}
}

type User struct {
	ID            string
	Email         string
	Name          string
	Role          string
	EmailVerified bool
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

// RedeemSetup creates the owner, an org, and recovery codes. Codes are returned once.
func (s *Service) RedeemSetup(ctx context.Context, code, email, name string) (User, []string, error) {
	email = emailNorm(email)
	if email == "" || code == "" {
		return User{}, nil, ErrSetupUsed
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return User{}, nil, err
	}
	defer tx.Rollback(ctx)
	var id string
	err = tx.QueryRow(ctx, `UPDATE identity_tokens SET used_at = now() WHERE kind = 'setup' AND hash = $1 AND used_at IS NULL AND expires_at > now() RETURNING id`, hashToken(code)).Scan(&id)
	if err != nil {
		return User{}, nil, ErrSetupUsed
	}
	user, err := insertUser(ctx, tx, email, name, "owner", true)
	if err != nil {
		return User{}, nil, err
	}
	org := uuid.Must(uuid.NewV7()).String()
	if _, err := tx.Exec(ctx, `INSERT INTO identity_orgs (id, name) VALUES ($1,$2)`, org, name+" org"); err != nil {
		return User{}, nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO identity_members (org_id, user_id, role) VALUES ($1,$2,'owner')`, org, user.ID); err != nil {
		return User{}, nil, err
	}
	codes, err := insertRecovery(ctx, tx, user.ID)
	if err != nil {
		return User{}, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, nil, err
	}
	s.audit(ctx, user.ID, "user.signup", map[string]string{"via": "setup"})
	s.emit(ctx, "user.signup", map[string]string{"user_id": user.ID, "via": "setup"})
	return user, codes, nil
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
		switch s.cfg.Directory.Signup {
		case "open":
		case "invite_only":
			if invite == "" {
				return ErrSignup
			}
		default:
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
		if s.cfg.Directory.Signup == "invite_only" {
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
	if s.cfg.Directory.Signup != "open" {
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
	return s.send.Send(ctx, email, "Confirm your request", "Confirm your waitlist request:\n"+raw+"\n")
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
}

func (s *Service) SearchWaitlist(ctx context.Context, q string) ([]WaitRow, error) {
	q = "%" + strings.TrimSpace(q) + "%"
	rows, err := s.db.Query(ctx, `SELECT id::text, email, name, company, notes, status FROM identity_waitlist WHERE email ILIKE $1 OR name ILIKE $1 ORDER BY created_at`, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WaitRow
	for rows.Next() {
		var w WaitRow
		if err := rows.Scan(&w.ID, &w.Email, &w.Name, &w.Company, &w.Notes, &w.Status); err != nil {
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

// BulkInvite invites confirmed waitlist rows and returns the raw codes.
func (s *Service) BulkInvite(ctx context.Context, actor string, ids []string) ([]string, error) {
	var codes []string
	for _, id := range ids {
		var email, status string
		err := s.db.QueryRow(ctx, `SELECT email, status FROM identity_waitlist WHERE id = $1`, id).Scan(&email, &status)
		if err != nil {
			return nil, err
		}
		if status != "confirmed" {
			return nil, errors.New("only confirmed rows can be invited")
		}
		code, err := s.CreateInvite(ctx, actor, email, "member", 1, s.now().Add(14*24*time.Hour))
		if err != nil {
			return nil, err
		}
		if _, err := s.db.Exec(ctx, `UPDATE identity_waitlist SET status = 'invited' WHERE id = $1`, id); err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	return codes, nil
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
