package api

import (
	"time"

	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/vaultix"
)

// vaultixDeps is the console's Vaultix (Infisical) client; nil while vaultix.enabled is false.
type vaultixDeps struct {
	c   *vaultix.Client
	cfg config.Vaultix
	db  *db.DB
}

func newVaultixDeps(cfg *config.Config, d *db.DB) *vaultixDeps {
	v := cfg.Vaultix
	if !v.Enabled {
		return nil
	}
	return &vaultixDeps{c: vaultix.New(v.APIURL, v.ClientID, v.ClientSecret, time.Duration(v.TimeoutMS)*time.Millisecond), cfg: v, db: d}
}
