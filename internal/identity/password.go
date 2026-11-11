package identity

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
	"shippin.cloud/console/internal/config"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,31}$`)
var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
var ErrPassword = errors.New("password must contain 12 or more characters and at most 1024 bytes")
var ErrBrand = errors.New("invalid branding")

func validUsername(s string) bool { return usernamePattern.MatchString(s) }

func hashPassword(password string) (string, error) {
	if utf8.RuneCountInString(password) < 12 || len(password) > 1024 {
		return "", ErrPassword
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, 2, 19*1024, 1, 32)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=19$m=19456,t=2,p=1$%s$%s", enc.EncodeToString(salt), enc.EncodeToString(hash)), nil
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" || parts[3] != "m=19456,t=2,p=1" {
		return false
	}
	enc := base64.RawStdEncoding
	salt, e1 := enc.DecodeString(parts[4])
	want, e2 := enc.DecodeString(parts[5])
	if e1 != nil || e2 != nil || len(salt) != 16 || len(want) != 32 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, 2, 19*1024, 1, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}

func validateBrand(b config.Brand) error {
	if strings.TrimSpace(b.Name) == "" || len(b.Name) > 80 || len(b.Short) > 24 || len(b.Tagline) > 160 {
		return fmt.Errorf("%w: name is required and text must fit its limits", ErrBrand)
	}
	if b.Color != "" && !colorPattern.MatchString(b.Color) {
		return fmt.Errorf("%w: color must use #RRGGBB format", ErrBrand)
	}
	for _, raw := range []string{b.LogoURL, b.PrivacyURL, b.TermsURL} {
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
			return fmt.Errorf("%w: URLs must be valid HTTPS URLs", ErrBrand)
		}
	}
	return nil
}

// AuthenticatePassword accepts a username or email and returns the same
// generic failure for unknown users and incorrect passwords.
func (s *Service) AuthenticatePassword(ctx context.Context, identifier, password, remote string) (User, error) {
	identifier = strings.ToLower(strings.TrimSpace(identifier))
	if s.cfg.Directory.UpstreamRequired || identifier == "" || len(password) > 1024 || !s.passwordLimit.Allow(remote, s.now()) {
		return User{}, ErrNotFound
	}
	var u User
	var encoded string
	err := s.db.QueryRow(ctx, `SELECT id,email,name,role,email_verified,COALESCE(username,''),password_hash FROM identity_users WHERE active AND (email=$1 OR lower(username)=$1)`, identifier).
		Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.EmailVerified, &u.Username, &encoded)
	if err != nil || encoded == "" {
		// Perform the same expensive work for unknown/passwordless accounts.
		_ = argon2.IDKey([]byte(password), make([]byte, 16), 2, 19*1024, 1, 32)
		return User{}, ErrNotFound
	}
	if !verifyPassword(encoded, password) {
		return User{}, ErrNotFound
	}
	return u, nil
}

// SetPassword lets an authenticated local account establish its first password
// or rotate one after proving the existing password. Passwordless accounts use
// their established session (for example after a passkey or recovery sign-in).
func (s *Service) SetPassword(ctx context.Context, userID, current, next, remote string) error {
	if len(current) > 1024 {
		return ErrNotFound
	}
	if !s.passwordLimit.Allow("change:"+remote, s.now()) {
		return ErrRateLimited
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var existing string
	if err := tx.QueryRow(ctx, `SELECT password_hash FROM identity_users WHERE id=$1 AND active FOR UPDATE`, userID).Scan(&existing); err != nil {
		return ErrNotFound
	}
	if existing != "" && !verifyPassword(existing, current) {
		return ErrNotFound
	}
	hash, err := hashPassword(next)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE identity_users SET password_hash=$2 WHERE id=$1 AND active`, userID, hash)
	if err != nil || tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.audit(ctx, userID, "user.password_changed", map[string]bool{"initial": existing == ""})
	return nil
}

// HashPassword encodes a password the way sign-in checks it.
func HashPassword(password string) (string, error) { return hashPassword(password) }
