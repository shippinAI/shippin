package api

import (
	"context"
	"log"
	"net/http"
	"os"

	"shippin.cloud/console/internal/clickhouse"
	"shippin.cloud/console/internal/platform"
	"shippin.cloud/console/internal/servers"
)

const clickHouseSealAAD = "platform-clickhouse"

// platformContext binds the platform context to this process.
func (a *API) platformContext(c *caller) *platform.Handler {
	deps := platform.Deps{
		DatabaseURL:   a.cfg.DatabaseURL,
		CacheURL:      a.cfg.RedisURL,
		CachePing:     a.cacheHealth,
		ClickHouse:    a.ch.Load,
		UseClickHouse: a.SetClickHouse,
		ClickHouseURL: a.cfg.ClickHouseURL,
		StateDir:      stateDir(),
		SessionKey:    a.cfg.SessionKey,
		Seal: func(plain []byte) ([]byte, error) {
			s, err := a.sealer()
			if err != nil {
				return nil, err
			}
			return s.Seal(plain, clickHouseSealAAD)
		},
		Unseal: func(box []byte) ([]byte, error) {
			s, err := a.sealer()
			if err != nil {
				return nil, err
			}
			return s.Open(box, clickHouseSealAAD)
		},
		Audit: func(w http.ResponseWriter, r *http.Request, action, detail string) bool {
			return a.auditOperator(w, r, c, servers.SystemTenantID, action, detail)
		},
	}
	if a.db != nil {
		deps.Pool = a.db.Pool
	}
	return &platform.Handler{Deps: deps}
}

func (a *API) platformContextRoutes() []opRoute {
	out := []opRoute{}
	for i, rt := range (&platform.Handler{}).Routes() {
		out = append(out, opRoute{rt.Pattern, rt.Perm, func(w http.ResponseWriter, r *http.Request, c *caller) {
			a.platformContext(c).Routes()[i].H(w, r, c.id.Sub)
		}})
	}
	return out
}

// stateDir is where an installation keeps files that must survive a rebuild.
func stateDir() string {
	if d := os.Getenv("SHIPPIN_STATE_DIR"); d != "" {
		return d
	}
	return "/var/lib/shippin"
}

// LoadPlatformServices applies connection choices an owner saved in the panel. A failure is logged and the
// panel still starts, without that service.
func (a *API) LoadPlatformServices(ctx context.Context) {
	if err := a.platformContext(nil).LoadClickHouse(ctx); err != nil {
		log.Printf("platform: saved ClickHouse connection not applied: %v", err)
	}
}

// SetClickHouse gives the API the metrics and events store. Nil means there is none.
func (a *API) SetClickHouse(c *clickhouse.Client) { a.ch.Store(c) }
