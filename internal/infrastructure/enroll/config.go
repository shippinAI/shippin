package enroll

import (
	"errors"
	"net/url"
	"path/filepath"
)

// Check (nil = enrollment off). The console URL must be https; plain http is accepted only for a
// loopback console (local tests), so a credential never crosses a network in clear.
func (c *Config) Check() error {
	if c == nil {
		return nil
	}
	u, err := url.Parse(c.ConsoleURL)
	if err != nil || u.Host == "" || u.Path != "" || (u.Scheme != "https" && !(u.Scheme == "http" && loopback(u.Hostname()))) {
		return errors.New("enroll.console_url must be an https origin (no path)")
	}
	if !filepath.IsAbs(c.TokenFile) || !filepath.IsAbs(c.CredentialFile) || c.TokenFile == c.CredentialFile {
		return errors.New("enroll.token_file and enroll.credential_file must be distinct absolute paths")
	}
	return nil
}

func loopback(h string) bool { return h == "127.0.0.1" || h == "::1" || h == "localhost" }
