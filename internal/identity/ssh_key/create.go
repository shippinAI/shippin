package ssh_key

import (
	"strings"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/hostd"
	"shippin.cloud/console/internal/mcpl"
)

// CreateInput is a key to add: a name for the person and one OpenSSH public key line.
type CreateInput struct {
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
}

var Create = mcpl.Def[CreateInput, Key]{
	Spec: mcpl.Spec{Summary: "Add an SSH public key to your account; it is put on your running workspaces too.", Access: mcpl.SignedIn},
	Run: func(c *mcpl.Call, in CreateInput) (Key, error) {
		in.PublicKey, in.Name = strings.TrimSpace(in.PublicKey), strings.TrimSpace(in.Name)
		if _, err := hostd.ValidKeys([]string{in.PublicKey}); err != nil || in.PublicKey == "" {
			return Key{}, mcpl.Invalid("That doesn't look like an OpenSSH public key.")
		}
		if in.Name == "" || len(in.Name) > 64 {
			return Key{}, mcpl.Invalid("Give the key a name (up to 64 characters).")
		}
		k := Key{Name: in.Name, PublicKey: in.PublicKey, Fingerprint: fingerprint(in.PublicKey)}
		var exists bool
		if err := c.DB.QueryRow(c.Ctx, `SELECT EXISTS(SELECT 1 FROM user_ssh_keys WHERE user_sub=$1 AND fingerprint=$2)`,
			c.Actor, k.Fingerprint).Scan(&exists); err != nil {
			return Key{}, err
		}
		if exists {
			return Key{}, mcpl.Conflict("This key is already on your account.")
		}
		id, _ := uuid.NewV7()
		k.ID = id.String()
		err := c.DB.QueryRow(c.Ctx, `INSERT INTO user_ssh_keys(id, user_sub, name, public_key, fingerprint) VALUES($1,$2,$3,$4,$5)
			RETURNING created_at`, k.ID, c.Actor, k.Name, k.PublicKey, k.Fingerprint).Scan(&k.CreatedAt)
		return k, err
	},
}
