package config

import "errors"

// Agents configures agent identities (N2; 02a-product-rbac: agents get their own identities,
// IdP service users now, Nomen token exchange with `act` later; never a user's raw token).
type Agents struct {
	Enabled bool `json:"enabled"`
	// MaxPerTenant caps live agent identities per tenant.
	MaxPerTenant int `json:"max_per_tenant"`
	// UserPrefix starts every agent's IdP user name (<prefix><agent uuid>).
	UserPrefix string `json:"user_prefix"`
}

// Check fails closed when agents are enabled without a cap or a prefix.
func (a Agents) Check() error {
	if !a.Enabled {
		return nil
	}
	if a.MaxPerTenant < 1 || a.UserPrefix == "" {
		return errors.New("agents: max_per_tenant >= 1 and user_prefix are required when enabled")
	}
	return nil
}
