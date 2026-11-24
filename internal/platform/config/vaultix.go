package config

import (
	"errors"
	"os"
)

// Vaultix is the secrets product over stock Infisical (03 decision 16, 08 VX1/VX2): the console
// calls Infisical's API with its own machine identity; browsers and agents never do.
type Vaultix struct {
	Enabled bool `json:"enabled"`
	// APIURL is Infisical's API base as the console reaches it (e.g. https://secrets.<domain>).
	APIURL string `json:"api_url"`
	// OrgID is the Infisical organization tenant projects are created in.
	OrgID string `json:"org_id"`
	// Envs are the environment slugs every tenant project gets, in tab order.
	Envs []string `json:"envs"`
	// ProjectPrefix names tenant projects (<prefix><tenant uuid>).
	ProjectPrefix string `json:"project_prefix"`
	TimeoutMS     int    `json:"timeout_ms"`
	// ClientID/ClientSecret are the console's Universal Auth identity (env only, never logged).
	ClientID     string `json:"-"`
	ClientSecret string `json:"-"`
}

func (v *Vaultix) load() {
	v.ClientID = os.Getenv("SHIPPIN_VAULTIX_CLIENT_ID")
	v.ClientSecret = os.Getenv("SHIPPIN_VAULTIX_CLIENT_SECRET")
}

// Check fails closed when Vaultix is on without a target, environments or a prefix.
func (v Vaultix) Check() error {
	if !v.Enabled {
		return nil
	}
	if v.APIURL == "" || v.OrgID == "" || len(v.Envs) == 0 || v.ProjectPrefix == "" || v.TimeoutMS < 1 {
		return errors.New("vaultix: api_url, org_id, envs, project_prefix and timeout_ms are required when enabled")
	}
	return nil
}

// HasEnv reports whether slug is one of the configured environments.
func (v Vaultix) HasEnv(slug string) bool {
	for _, e := range v.Envs {
		if e == slug {
			return true
		}
	}
	return false
}
