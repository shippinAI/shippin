package config

import (
	"log"
	"os"
	"regexp"
	"strings"
)

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

var tenantIDRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// loadEnv adds SHIPPIN_RRFLOW_TENANTS (comma-separated tenant UUIDs from the console env file) to
// the rollout list, so a change ticket can enable rrflow for a tenant whose id is only known on the
// host (a dry-run tenant) without a new release. pa-pb-rev1 named one such tenant; it is not stored
// in shippin.json. Set it in the env file (see config/rrflow-tenants.env.example). Invalid entries
// are dropped and logged by position.
func (s *ServiceToggle) loadEnv() {
	for i, t := range strings.Split(os.Getenv("SHIPPIN_RRFLOW_TENANTS"), ",") {
		switch t = strings.TrimSpace(t); {
		case t == "":
		case tenantIDRe.MatchString(t):
			s.Tenants = append(s.Tenants, t)
		default:
			log.Printf("config: SHIPPIN_RRFLOW_TENANTS entry %d is not a tenant id; ignored", i+1)
		}
	}
}
