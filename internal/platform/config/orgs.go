package config

import "errors"

// Orgs configures the CP-P0b org split: the single Operations org and support grants.
// Off by default. BootstrapAdminSub (a Nomen subject, not a secret) is made ops_admin on first use
// so the first staff member needs no database access; later staff are granted by an ops_admin.
type Orgs struct {
	Enabled                bool   `json:"enabled"`
	OperationsSlug         string `json:"operations_slug"`
	OperationsName         string `json:"operations_name"`
	OperationsNomenOrgID   string `json:"operations_nomen_org_id"`
	BootstrapAdminSub      string `json:"bootstrap_admin_sub"`
	SupportGrantMaxSeconds int    `json:"support_grant_max_seconds"`
}

// Check fails closed when enabled.
func (o Orgs) Check() error {
	if !o.Enabled {
		return nil
	}
	if o.OperationsSlug == "" || o.OperationsName == "" {
		return errors.New("orgs: operations_slug and operations_name are required")
	}
	if o.SupportGrantMaxSeconds < 300 || o.SupportGrantMaxSeconds > 7*86400 {
		return errors.New("orgs.support_grant_max_seconds must be 300..604800")
	}
	return nil
}
