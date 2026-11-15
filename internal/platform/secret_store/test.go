package secret_store

import (
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/ports"
)

// Test is shippin.platform.secret_store.test: POST /v1/operator/platform/secret-store/test. It checks
// the saved connection again and records the result; a failed check is a result, not an error.
var Test = mcpl.Def[mcpl.None, Connection]{
	Spec: mcpl.Spec{Summary: "Check that the panel can still sign in to its secret store and write, read and delete a value there.", Access: mcpl.Staff, Perm: "platform.settings.write", Singleton: true},
	Run: func(c *mcpl.Call, _ mcpl.None) (Connection, error) {
		s, err := read(c.Ctx, c.DB, true)
		if err != nil {
			return Connection{}, err
		}
		if !s.Connected {
			return Connection{}, mcpl.Conflict(ports.ErrNoSecretStore.Error())
		}
		seal, err := mcpl.Need[ports.Sealer](c)
		if err != nil {
			return Connection{}, err
		}
		msg := ""
		if plain, err := seal.Open(s.sealed, aad); err != nil {
			msg = "the saved client secret can't be opened with this panel's key; enter it again"
		} else if err := Check(c.Ctx, Fields{APIURL: s.APIURL, ProjectID: s.ProjectID, Environment: s.Environment, Path: s.Path, ClientID: s.ClientID}, string(plain)); err != nil {
			msg = err.Error()
		}
		if len(msg) > 1000 {
			msg = msg[:1000]
		}
		if _, err := c.DB.Exec(c.Ctx, `UPDATE shippin_secret_store SET checked_at=now(), check_error=$1 WHERE id`, msg); err != nil {
			return Connection{}, err
		}
		s, err = read(c.Ctx, c.DB, false)
		return s.Connection, err
	},
}
