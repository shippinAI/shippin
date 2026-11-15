package secret_store

import (
	"fmt"

	"shippin.cloud/console/internal/mcpl"
)

// Delete is shippin.platform.secret_store.delete: DELETE /v1/operator/platform/secret-store. It
// disconnects the store; the keys in Vaultix stay there. It is refused while settings keep keys in it,
// since the panel could no longer read them.
var Delete = mcpl.Def[mcpl.None, mcpl.None]{
	Spec: mcpl.Spec{Summary: "Disconnect the secret store, once no setting keeps a key there.", Access: mcpl.Staff, Perm: "platform.settings.write", Singleton: true},
	Run: func(c *mcpl.Call, _ mcpl.None) (mcpl.None, error) {
		s, err := read(c.Ctx, c.DB, true)
		switch {
		case err != nil:
			return mcpl.None{}, err
		case !s.Connected:
			return mcpl.None{}, mcpl.NotFound("No secret store is connected.")
		case s.KeysHeld > 0:
			return mcpl.None{}, mcpl.Conflict(fmt.Sprintf("%d settings sections keep keys in this store. Remove those keys first, or the panel can't read them.", s.KeysHeld))
		}
		_, err = c.DB.Exec(c.Ctx, `DELETE FROM shippin_secret_store WHERE id`)
		return mcpl.None{}, err
	},
}
