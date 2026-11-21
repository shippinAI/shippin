package secret_store

import (
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/ports"
)

// Update is shippin.platform.secret_store.update: PUT /v1/operator/platform/secret-store. It checks
// the connection first, signing in and writing, reading and deleting a check value, and saves it only
// if that works. While sections keep keys in the store, only the identity may change, not where the
// keys are.
var Update = mcpl.Def[Fields, Connection]{
	Spec: mcpl.Spec{Summary: "Connect this installation to its Vaultix account with a machine identity, or change the identity.", Access: mcpl.Staff, Perm: "platform.settings.write", Singleton: true},
	Run: func(c *mcpl.Call, in Fields) (Connection, error) {
		if err := in.check(); err != nil {
			return Connection{}, err
		}
		seal, err := mcpl.Need[ports.Sealer](c)
		if err != nil {
			return Connection{}, err
		}
		old, err := read(c.Ctx, c.DB, true)
		if err != nil {
			return Connection{}, err
		}
		if old.KeysHeld > 0 && (old.APIURL != in.APIURL || old.ProjectID != in.ProjectID || old.Environment != in.Environment || old.Path != in.Path) {
			return Connection{}, mcpl.Conflict("Settings keep keys in this store, so its address, project, environment and folder can't change. Only the identity can.")
		}
		secret := in.ClientSecret
		sealed := old.sealed
		switch {
		case secret != "":
			if sealed, err = seal.Seal([]byte(secret), aad); err != nil {
				return Connection{}, err
			}
		case !old.Connected || old.ClientID != in.ClientID:
			return Connection{}, mcpl.Invalid("Enter the machine identity's client secret.")
		default:
			plain, err := seal.Open(old.sealed, aad)
			if err != nil {
				return Connection{}, mcpl.Invalid("The saved client secret can't be opened with this panel's key. Enter it again.")
			}
			secret = string(plain)
		}
		if err := Check(c.Ctx, in, secret); err != nil {
			return Connection{}, mcpl.Invalid("Not saved: " + err.Error() + ".")
		}
		if _, err := c.DB.Exec(c.Ctx, `INSERT INTO shippin_secret_store(id, api_url, project_id, environment, path, client_id, client_secret, updated_by, checked_at, check_error)
		  VALUES (true,$1,$2,$3,$4,$5,$6,$7,now(),'')
		  ON CONFLICT (id) DO UPDATE SET api_url=EXCLUDED.api_url, project_id=EXCLUDED.project_id, environment=EXCLUDED.environment, path=EXCLUDED.path,
		    client_id=EXCLUDED.client_id, client_secret=EXCLUDED.client_secret, updated_by=EXCLUDED.updated_by, updated_at=now(), checked_at=now(), check_error=''`,
			in.APIURL, in.ProjectID, in.Environment, in.Path, in.ClientID, sealed, c.Actor); err != nil {
			return Connection{}, err
		}
		s, err := read(c.Ctx, c.DB, false)
		return s.Connection, err
	},
}
