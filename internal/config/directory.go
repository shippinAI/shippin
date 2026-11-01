package config

import (
	"errors"
	"strings"
)

// Directory is the local account policy. Empty fields are filled by DirectoryDefaults.
type Directory struct {
	Signup            string     `json:"signup"` // open, invite_only, waitlist
	UpstreamRequired  bool       `json:"upstream_required"`
	RPID              string     `json:"rp_id"`
	Origin            string     `json:"origin"`
	ClientID          string     `json:"client_id"`
	Redirects         []string   `json:"redirects"`
	Captcha           Captcha    `json:"captcha"`
	Upstreams         []Upstream `json:"upstreams"`
	WebhookSkewSecond int        `json:"webhook_skew_seconds"`
}

// Captcha selects the public-form check. Provider none means the site does not run one.
type Captcha struct {
	Provider  string `json:"provider"` // none or turnstile
	SecretEnv string `json:"secret_env"`
}

// Upstream is a discovered OIDC issuer. Empty issuer or client_id means "not configured".
type Upstream struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Issuer    string `json:"issuer"`
	ClientID  string `json:"client_id"`
	SecretEnv string `json:"secret_env"`
}

// DirectoryDefaults fills the policy a site left blank.
func (c *Config) DirectoryDefaults() {
	d := &c.Directory
	if d.Signup == "" {
		d.Signup = "invite_only"
	}
	if d.ClientID == "" {
		d.ClientID = "shippin-panel"
	}
	if d.Captcha.Provider == "" {
		d.Captcha.Provider = "none"
	}
	if d.Captcha.SecretEnv == "" {
		d.Captcha.SecretEnv = "SHIPPIN_CAPTCHA_SECRET"
	}
	if d.WebhookSkewSecond == 0 {
		d.WebhookSkewSecond = 300
	}
	if d.RPID == "" {
		d.RPID = c.ConsoleHost
	}
	if d.Origin == "" && c.ConsoleHost != "" {
		d.Origin = "https://" + c.ConsoleHost
	}
	if len(d.Redirects) == 0 && c.ConsoleHost != "" {
		d.Redirects = []string{"https://" + c.ConsoleHost + "/auth/callback"}
	}
}

// Configured reports whether the upstream has the two values a site must set.
func (u Upstream) Configured() bool {
	return strings.TrimSpace(u.Issuer) != "" && strings.TrimSpace(u.ClientID) != ""
}

// Check rejects an unknown signup mode or a required upstream that was not configured.
func (d Directory) Check(idpClient string) error {
	switch d.Signup {
	case "open", "invite_only", "waitlist":
	default:
		return errors.New("directory.signup must be open, invite_only, or waitlist")
	}
	switch d.Captcha.Provider {
	case "none", "turnstile":
	default:
		return errors.New("directory.captcha.provider must be none or turnstile")
	}
	if d.UpstreamRequired && idpClient == "" && !anyUpstream(d.Upstreams) {
		return errors.New("directory.upstream_required needs an issuer client id")
	}
	if d.WebhookSkewSecond < 30 || d.WebhookSkewSecond > 3600 {
		return errors.New("directory.webhook_skew_seconds must be 30..3600")
	}
	return nil
}

func anyUpstream(us []Upstream) bool {
	for _, u := range us {
		if u.Configured() {
			return true
		}
	}
	return false
}
