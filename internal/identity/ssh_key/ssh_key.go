// Package ssh_key is a person's SSH public keys: Shippin puts them on the workspaces that person
// creates. Its actions are list, create and delete under /v1/me/identity/ssh-keys.
package ssh_key

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"time"

	"shippin.cloud/console/internal/mcpl"
)

// Key is one public key on a person's account.
type Key struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	PublicKey   string    `json:"public_key"`
	Fingerprint string    `json:"fingerprint"`
	CreatedAt   time.Time `json:"created_at"`
}

// ByID names one of the caller's keys.
type ByID struct {
	ID string `path:"sshKeyId" json:"-"`
}

// Of lists a person's keys, oldest first.
func Of(ctx context.Context, q mcpl.DB, sub string) ([]Key, error) {
	rows, err := q.Query(ctx, `SELECT id::text, name, public_key, fingerprint, created_at FROM user_ssh_keys
		WHERE user_sub=$1 ORDER BY created_at`, sub)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Key{}
	for rows.Next() {
		var k Key
		if err := rows.Scan(&k.ID, &k.Name, &k.PublicKey, &k.Fingerprint, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// PublicKeys are a person's keys as authorized_keys lines, for a workspace.
func PublicKeys(ctx context.Context, q mcpl.DB, sub string) ([]string, error) {
	ks, err := Of(ctx, q, sub)
	out := make([]string, 0, len(ks))
	for _, k := range ks {
		out = append(out, k.PublicKey)
	}
	return out, err
}

// fingerprint is the key's SHA256 fingerprint as ssh-keygen -l prints it.
func fingerprint(pub string) string {
	f := strings.Fields(pub)
	if len(f) < 2 {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(f[1])
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return "SHA256:" + base64.RawStdEncoding.EncodeToString(sum[:])
}
