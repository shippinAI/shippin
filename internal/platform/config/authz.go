package config

import (
	"errors"
	"os"
)

// Authz configures the OpenFGA check path (02a: ReBAC behind one Authorize interface).
// Mode "off" skips it, "shadow" runs every role decision through OpenFGA as well and records
// agreement without changing the outcome, "enforce" is reserved until shadow shows no drift.
type Authz struct {
	Mode    string `json:"mode"`
	URL     string `json:"url"`      // OpenFGA HTTP API, e.g. http://203.0.113.80:8080
	StoreID string `json:"store_id"` // set by deploy/openfga/load-model.sh
	ModelID string `json:"model_id"` // pinned authorization model id
	// OwnershipModelID is the model project-ownership tuples are written against (R-J1a: org,
	// devspace, rrflow_namespace). SHIPPIN_FGA_OWNERSHIP_MODEL_ID overrides; empty = the store's latest.
	OwnershipModelID string `json:"ownership_model_id,omitempty"`
	// TimeoutMS bounds one check; a slow or failed check never blocks a request in shadow mode.
	TimeoutMS int `json:"timeout_ms"`
	// Token is the OpenFGA preshared key (SHIPPIN_FGA_TOKEN), never in the config file.
	Token string `json:"-"`
}

func (a *Authz) load() {
	a.Token = os.Getenv("SHIPPIN_FGA_TOKEN")
	if v := os.Getenv("SHIPPIN_FGA_OWNERSHIP_MODEL_ID"); v != "" {
		a.OwnershipModelID = v
	}
}

// Check fails closed on a mode it doesn't know or an incomplete OpenFGA target.
func (a Authz) Check() error {
	switch a.Mode {
	case "", "off":
		return nil
	case "shadow":
	case "enforce":
		return errors.New("authz: enforce mode is not available yet; use shadow")
	default:
		return errors.New("authz: mode must be off or shadow")
	}
	if a.URL == "" || a.StoreID == "" || a.ModelID == "" || a.TimeoutMS < 1 {
		return errors.New("authz: url, store_id, model_id and timeout_ms are required in shadow mode")
	}
	return nil
}
