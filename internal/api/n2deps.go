package api

import (
	"log"
	"shippin.cloud/console/internal/chat"
	"time"

	"shippin.cloud/console/internal/authz"
	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
)

// n2Deps holds the N2 integrations: the OpenFGA shadow and the Vaultix (Infisical) client.
type n2Deps struct {
	fga *authz.Shadow
	tw  authz.Writer // ownership tuples (R-J1a); set whenever OpenFGA is configured
	vx  *vaultixDeps
	br  *bridgeDeps
	ch  *chat.Store
}

func newN2(cfg *config.Config, d *db.DB) n2Deps {
	var n n2Deps
	if a := cfg.Authz; a.Mode == "shadow" {
		t := time.Duration(a.TimeoutMS) * time.Millisecond
		n.fga = &authz.Shadow{A: authz.NewFGA(a.URL, a.StoreID, a.ModelID, a.Token, t), Timeout: t}
		log.Printf("authz: OpenFGA shadow on (model %s)", a.ModelID)
	}
	if a := cfg.Authz; a.URL != "" && a.StoreID != "" {
		n.tw = authz.NewFGA(a.URL, a.StoreID, a.OwnershipModelID, a.Token, time.Duration(max(a.TimeoutMS, 3000))*time.Millisecond)
	}
	n.vx = newVaultixDeps(cfg, d)
	n.br = newBridgeDeps(cfg.Bridge, d)
	if cfg.Chat.Enabled {
		n.ch = &chat.Store{Pool: d.Pool}
	}
	return n
}
