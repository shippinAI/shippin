package config

import "errors"

// Bridge is devspace access through the Shippin MCP bridge (N4 SSH share, the operator 2026-10-19):
// device-code sign-in bound to the account, short-lived SSH certificates from the Shippin CA
// (principal = workspace) and a relay through hostd. No static keys, no inbound ports.
type Bridge struct {
	Enabled bool `json:"enabled"`
	// CAKeyFile is the SSH user CA private key (OpenSSH format, 0600, never logged).
	CAKeyFile string `json:"ca_key_file"`
	// CertTTLSeconds bounds every certificate; SessionHours bounds a device sign-in.
	CertTTLSeconds  int `json:"cert_ttl_seconds"`
	SessionHours    int `json:"session_hours"`
	DeviceTTLSecond int `json:"device_ttl_seconds"`
	PollSeconds     int `json:"poll_seconds"`
	// RecheckSeconds: an open relay re-checks the session and RBAC this often and closes on deny.
	RecheckSeconds int `json:"recheck_seconds"`
	// SSHUser is the workspace login user; PrincipalPrefix + workspace UUID is the cert principal.
	SSHUser         string `json:"ssh_user"`
	PrincipalPrefix string `json:"principal_prefix"`
	// TokenPrefix marks bridge bearer tokens so they never reach the IdP as access tokens.
	TokenPrefix string `json:"token_prefix"`
}

// Check fails closed when the bridge is on without its CA or limits.
func (b Bridge) Check() error {
	if !b.Enabled {
		return nil
	}
	if b.CAKeyFile == "" || b.SSHUser == "" || b.PrincipalPrefix == "" || b.TokenPrefix == "" {
		return errors.New("bridge: ca_key_file, ssh_user, principal_prefix and token_prefix are required")
	}
	if b.CertTTLSeconds < 60 || b.CertTTLSeconds > 3600 || b.SessionHours < 1 || b.SessionHours > 24 ||
		b.DeviceTTLSecond < 60 || b.PollSeconds < 1 || b.RecheckSeconds < 5 {
		return errors.New("bridge: cert_ttl_seconds 60..3600, session_hours 1..24, device_ttl_seconds >= 60, poll_seconds >= 1, recheck_seconds >= 5")
	}
	return nil
}
