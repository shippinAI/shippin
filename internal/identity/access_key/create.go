package access_key

import (
	"strings"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/mcpl"
)

// CreateInput names the key and how many days it lasts (1 to 90; 30 when left out).
type CreateInput struct {
	Name string `json:"name"`
	Days int    `json:"days,omitempty"`
}

// Created is the new key, with the key itself: shown this once and never again.
type Created struct {
	Key
	Token string `json:"token"`
}

var Create = mcpl.Def[CreateInput, Created]{
	Spec: mcpl.Spec{Summary: "Make an access key an AI client or script uses to act as you; it is shown once.", Access: mcpl.SignedIn},
	Run: func(c *mcpl.Call, in CreateInput) (Created, error) {
		user := localUser(c.Actor)
		switch {
		case user == "":
			return Created{}, mcpl.Forbidden("You sign in through an identity provider; use a token from it instead.")
		case c.Via == "access_key":
			return Created{}, mcpl.Forbidden("An access key can't make another one. Make it from a signed-in session.")
		}
		in.Name = strings.TrimSpace(in.Name)
		if in.Name == "" || len(in.Name) > 64 {
			return Created{}, mcpl.Invalid("Name the key after what will use it (up to 64 characters).")
		}
		if in.Days == 0 {
			in.Days = 30
		}
		if in.Days < 1 || in.Days > 90 {
			return Created{}, mcpl.Invalid("A key lasts from 1 to 90 days.")
		}
		var n int
		if err := c.DB.QueryRow(c.Ctx, `SELECT count(*) FROM access_keys WHERE user_id::text = $1 AND expires_at > now()`, user).Scan(&n); err != nil {
			return Created{}, err
		}
		if n >= 20 {
			return Created{}, mcpl.Conflict("You have 20 keys already. Revoke one you no longer use.")
		}
		id, _ := uuid.NewV7()
		token := newToken()
		k := Created{Key: Key{ID: id.String(), Name: in.Name, Prefix: token[:len(Prefix)+6]}, Token: token}
		method := c.Via
		if method == "" {
			method = "password"
		}
		err := c.DB.QueryRow(c.Ctx, `INSERT INTO access_keys(id, user_id, name, prefix, hash, method, expires_at)
			VALUES($1, $2, $3, $4, $5, $6, now() + make_interval(days => $7)) RETURNING created_at, expires_at`,
			k.ID, user, k.Name, k.Prefix, hash(token), method, in.Days).Scan(&k.CreatedAt, &k.ExpiresAt)
		return k, err
	},
}
