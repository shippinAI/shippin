package config

import (
	"errors"
	"time"
)

// Staff configures provider-panel staff roles. Roles, their permissions and the team settings
// are data (Postgres, edited in the Team page); this only says where the IdP keeps grants and
// offers a rollback switch.
type Staff struct {
	// ProjectID is the Nomen project whose roles are staff roles.
	ProjectID string `json:"project_id"`
	// OrgID is the operator org: only grants made by this org count. Empty means
	// operator.platform_org_id.
	OrgID string `json:"org_id"`
	// Mode, when set, overrides the mode stored in the Team settings (legacy, shadow, enforce).
	// It is the rollback switch: "legacy" restores the single IAM gate exactly.
	Mode string `json:"mode"`
	// CacheSeconds caches each user's resolved staff roles (default 30).
	CacheSeconds int `json:"cache_seconds"`
	// TeamToken is the console-team machine user's PAT (SHIPPIN_IDP_TEAM_TOKEN).
	TeamToken string `json:"-"`
	// Session is staff mode: a server-side session opened after a fresh IdP sign-in.
	Session StaffSession `json:"session"`
}

// StaffSession: Minutes is the session length; ReauthMinutes how recent the IdP sign-in must be
// to open one; Required makes every permissioned operator route need an open session.
type StaffSession struct {
	Minutes            int  `json:"minutes"`
	ReauthMinutes      int  `json:"reauth_minutes"`
	LocalReauthMinutes int  `json:"local_reauth_minutes"`
	Required           bool `json:"required"`
}

// Modes are the authorization modes, in rollout order.
var Modes = []string{"legacy", "shadow", "enforce"}

// ValidMode reports whether m is a known mode.
func ValidMode(m string) bool {
	for _, x := range Modes {
		if x == m {
			return true
		}
	}
	return false
}

// Check fails closed on an unknown override.
func (s Staff) Check() error {
	if s.Mode != "" && !ValidMode(s.Mode) {
		return errors.New("staff.mode must be legacy, shadow or enforce")
	}
	if s.CacheSeconds < 0 || s.CacheSeconds > 600 {
		return errors.New("staff.cache_seconds must be 0..600")
	}
	if s.Session.Minutes < 0 || s.Session.Minutes > 720 || s.Session.ReauthMinutes < 0 || s.Session.ReauthMinutes > 60 {
		return errors.New("staff.session needs minutes 0..720 and reauth_minutes 0..60 (0 = default)")
	}
	return nil
}

// StaffOrg is the operator org whose grants count.
func (c *Config) StaffOrg() string {
	if c.Staff.OrgID != "" {
		return c.Staff.OrgID
	}
	return c.Operator.PlatformOrgID
}

// TTL is the staff-mode session length (default 60 minutes).
func (s StaffSession) TTL() time.Duration { return minutesOr(s.Minutes, 60) }

// Reauth is how recent the IdP sign-in must be to open a session (default 5 minutes).
func (s StaffSession) Reauth() time.Duration { return minutesOr(s.ReauthMinutes, 5) }

// LocalReauth is the same for the local installation owner (default 12 hours, the life of a local
// session). They may have no mail or passkey, and each recovery code works only once.
func (s StaffSession) LocalReauth() time.Duration { return minutesOr(s.LocalReauthMinutes, 720) }

func minutesOr(n, def int) time.Duration {
	if n <= 0 {
		n = def
	}
	return time.Duration(n) * time.Minute
}
