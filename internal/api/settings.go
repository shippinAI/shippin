package api

import (
	"context"

	"shippin.cloud/console/internal/ext"
	"shippin.cloud/console/internal/platform/secret_store"
	"shippin.cloud/console/internal/ports"
	"shippin.cloud/console/internal/settings"
)

// newSettings is the installation's settings (docs/settings.md): every generated section, seeded from
// the bootstrap file, with keys in the connected secret store. A section that describes itself wrongly
// is a programming error caught by tests, so it stops the panel.
func (a *API) newSettings() {
	var pool = a.db.Pool
	a.secrets = &secret_store.Vaultix{Pool: pool}
	if seal, err := a.sealer(); err == nil {
		a.secrets.Sealer = seal
	}
	st, err := settings.New(pool, a.cfg, ext.Settings(), a.secretStore)
	if err != nil {
		panic(err)
	}
	a.settings = st
}

// secretStore is ports.Secrets, or nil when this panel has no at-rest key to open the store's client
// secret with: then keys can't be saved and the form says so.
func (a *API) secretStore() ports.Secrets {
	if a.secrets == nil || a.secrets.Sealer == nil {
		return nil
	}
	return a.secrets
}

// RunSettings deletes keys replaced in the secret store until ctx ends.
func (a *API) RunSettings(ctx context.Context) { a.settings.Run(ctx) }
