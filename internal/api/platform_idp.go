package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"

	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
)

// The IdP issuer is ops-editable like the other platform settings, but the OIDC client is built
// once at start, so an override takes effect when the console restarts (ApplyIssuerOverride).
// It must be https on one of the governance domains: the issuer decides who can sign in.
func issuerKey(cfg *config.Config) platformKey {
	return platformKey{Key: "idp.issuer", Kind: "string", Label: "IdP issuer URL",
		Help: "OIDC issuer the console signs users in with (https, on " + strings.Join(cfg.GovernanceDomains, ", ") +
			"). Applies when the console restarts; clearing it returns to the config file value.",
		def:   func(a *API) any { return cfg.IDP.Issuer },
		valid: func(v any) error { s, _ := v.(string); return validIssuer(s, cfg.GovernanceDomains) }}
}

func validIssuer(s string, domains []string) error {
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Port() != "" || len(s) > 200 {
		return fmt.Errorf("must be an https URL like https://id.example.com (no port, query or credentials)")
	}
	h := strings.ToLower(u.Hostname())
	for _, d := range domains {
		if d = strings.ToLower(d); h == d || strings.HasSuffix(h, "."+d) {
			return nil
		}
	}
	return fmt.Errorf("host must be on %s", strings.Join(domains, ", "))
}

// ApplyIssuerOverride sets cfg.IDP.Issuer from the idp.issuer platform setting, if one is stored
// and valid; called by cmd/console before the OIDC flow is built. An invalid row is ignored.
func ApplyIssuerOverride(ctx context.Context, cfg *config.Config, d *db.DB) {
	m, err := d.PlatformSettings(ctx)
	if err != nil {
		log.Printf("idp.issuer override not read (%v); using the config file", err)
		return
	}
	var v string
	if raw, ok := m["idp.issuer"]; !ok || json.Unmarshal(raw, &v) != nil || v == "" {
		return
	}
	if err := validIssuer(v, cfg.GovernanceDomains); err != nil {
		log.Printf("idp.issuer override ignored: %v", err)
		return
	}
	log.Printf("idp.issuer: using the ops override (config file: %s)", cfg.IDP.Issuer)
	cfg.IDP.Issuer = v
}
