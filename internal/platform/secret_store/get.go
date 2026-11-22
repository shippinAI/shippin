package secret_store

import "shippin.cloud/console/internal/mcpl"

// Get is shippin.platform.secret_store.get: GET /v1/operator/platform/secret-store. It says whether a
// store is connected, where, and how its last check went; never the client secret.
var Get = mcpl.Def[mcpl.None, Connection]{
	Spec: mcpl.Spec{Summary: "See where this installation keeps the keys entered in the panel.", Access: mcpl.Staff, Perm: "platform.settings.read", Singleton: true},
	Run: func(c *mcpl.Call, _ mcpl.None) (Connection, error) {
		s, err := read(c.Ctx, c.DB, false)
		return s.Connection, err
	},
}
