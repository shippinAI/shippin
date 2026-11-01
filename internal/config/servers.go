package config

import "errors"

// Servers configures the server registry (CP-P1): enrollment tokens, host credentials and the
// heartbeat that decides active vs stale. All durations are seconds.
type Servers struct {
	Enabled bool `json:"enabled"`
	// TokenTTLSeconds is how long a single-use enrollment token stays redeemable.
	TokenTTLSeconds int `json:"token_ttl_seconds"`
	// HeartbeatSeconds is the beat interval hostd uses (the console tells it on enroll).
	HeartbeatSeconds int `json:"heartbeat_seconds"`
	// MissedBeats: an active server with no beat for MissedBeats*HeartbeatSeconds shows as stale.
	MissedBeats int `json:"missed_beats"`
	// CredentialTTLSeconds: each beat extends the host credential to now+TTL; a host silent for
	// longer must be re-enrolled with a fresh token.
	CredentialTTLSeconds int `json:"credential_ttl_seconds"`
	// MaxPerTenant caps servers per tenant.
	MaxPerTenant int `json:"max_per_tenant"`
}

// Check fails closed: every value is required when enabled, and the credential must outlive the
// stale window so a stale host can still come back by itself.
func (s Servers) Check() error {
	if !s.Enabled {
		return nil
	}
	if s.TokenTTLSeconds < 60 || s.TokenTTLSeconds > 86400 {
		return errors.New("servers.token_ttl_seconds must be 60..86400")
	}
	if s.HeartbeatSeconds < 1 || s.MissedBeats < 1 || s.MaxPerTenant < 1 {
		return errors.New("servers: heartbeat_seconds, missed_beats and max_per_tenant must be >= 1")
	}
	if s.CredentialTTLSeconds <= s.HeartbeatSeconds*s.MissedBeats {
		return errors.New("servers.credential_ttl_seconds must exceed heartbeat_seconds*missed_beats")
	}
	return nil
}
