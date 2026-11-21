// Package access_key is a person's access keys: what an AI client or a script uses to act as that
// person through MCP or the API, with their permissions, audit entries and events. Keys belong to
// accounts in this panel's own directory; people who sign in through an identity provider use its
// tokens instead.
package access_key

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"strings"
	"time"

	"shippin.cloud/console/internal/mcpl"
)

// Prefix starts every key, so it is recognised (and refused by secret scanners) on sight.
const Prefix = "shp_"

// Key is an access key as its owner sees it: never the key itself, which is shown once.
type Key struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

// ByID names one of the caller's keys.
type ByID struct {
	ID string `path:"accessKeyId" json:"-"`
}

// localUser is the directory user ID behind an actor, or "" for an identity provider's subject.
func localUser(actor string) string {
	if id, ok := strings.CutPrefix(actor, "local:"); ok {
		return id
	}
	return ""
}

func hash(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func newToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return Prefix + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
}

// Resolve is the person and sign-in method a key acts as, when it is current and its account active.
// It notes the key's use at most once a minute.
func Resolve(ctx context.Context, q mcpl.DB, token string) (userID, method string, ok bool) {
	if !strings.HasPrefix(token, Prefix) || len(token) > 128 {
		return "", "", false
	}
	h := hash(token)
	err := q.QueryRow(ctx, `SELECT k.user_id::text, k.method FROM access_keys k JOIN identity_users u ON u.id = k.user_id
		WHERE k.hash = $1 AND k.expires_at > now() AND u.active`, h).Scan(&userID, &method)
	if err != nil {
		return "", "", false
	}
	_, _ = q.Exec(ctx, `UPDATE access_keys SET last_used_at = now() WHERE hash = $1 AND (last_used_at IS NULL OR last_used_at < now() - interval '1 minute')`, h)
	return userID, method, true
}
