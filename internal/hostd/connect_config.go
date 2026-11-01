package hostd

import (
	"fmt"
	"regexp"
	"strings"
)

// ConnectConfig drives "Connect accounts": per-provider CLI logins run inside the workspace and
// the per-workspace credential writer. Every argv, path, env name and pattern comes from config;
// an empty writer_unit disables the routes.
type ConnectConfig struct {
	// Env is set on every CLI run (e.g. CLAUDE_CONFIG_DIR, GROK_HOME, GH_CONFIG_DIR under ~/.ai).
	Env        map[string]string          `json:"env"`
	Providers  map[string]ConnectProvider `json:"providers"`
	SpoolDir   string                     `json:"spool_dir"`   // tmpfs drop box hostd writes tokens into
	StoreDir   string                     `json:"store_dir"`   // persistent store on the AI home
	StatusPath string                     `json:"status_path"` // writer's status report
	WriterUnit string                     `json:"writer_unit"`
	WriterBin  string                     `json:"writer_bin"`  // host path of shippin-credwriter
	WriterPath string                     `json:"writer_path"` // its path inside the workspace
	// LoginSeconds bounds one browser sign-in; OutputMax bounds the CLI output hostd scans.
	LoginSeconds int `json:"login_seconds"`
	OutputMax    int `json:"output_max_bytes"`
	StatusMax    int `json:"status_max_bytes"`
	// RefreshSeconds is the writer's loop: ingest, refresh, report.
	RefreshSeconds int `json:"refresh_seconds"`
	// ProfilePath is the login-shell hook that sources the writer's env file.
	ProfilePath string `json:"profile_path"`
}

// ConnectProvider is one CLI. Login is the browser/device argv; TokenEnv names the env var a
// pasted token is exposed as; TokenLogin (optional) reads the token on stdin to sign the CLI in.
type ConnectProvider struct {
	Label        string   `json:"label"`
	Login        []string `json:"login"`
	Logout       []string `json:"logout"`
	Status       []string `json:"status"`
	Refresh      []string `json:"refresh"`
	TokenEnv     string   `json:"token_env"`
	TokenLogin   []string `json:"token_login"`
	TokenPattern string   `json:"token_pattern"`
	URLPrefixes  []string `json:"url_prefixes"`
	CodePattern  string   `json:"code_pattern"`
	// PressEnter answers the CLI's "press Enter to open the browser" prompt; CodeInput means the
	// vendor shows a code in the browser that the user pastes back (no RFC 8628 device flow).
	PressEnter bool `json:"press_enter"`
	CodeInput  bool `json:"code_input"`
}

var envNameRe = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

func (c ConnectConfig) enabled() bool { return c.WriterUnit != "" }

func (c ConnectConfig) check() error {
	if !c.enabled() {
		return nil
	}
	for _, p := range []string{c.SpoolDir, c.StatusPath} {
		if !strings.HasPrefix(p, "/run/") || strings.Contains(p, "..") {
			return fmt.Errorf("connect.spool_dir and status_path must be under /run")
		}
	}
	if !strings.HasPrefix(c.StoreDir, "/home/") || !strings.HasPrefix(c.WriterPath, "/home/") || !unitRe.MatchString(c.WriterUnit) {
		return fmt.Errorf("connect.store_dir and writer_path must be under /home and writer_unit a .service")
	}
	if c.LoginSeconds < 30 || c.OutputMax < 1024 || c.StatusMax < 256 || c.WriterBin == "" || c.RefreshSeconds < 10 ||
		!strings.HasPrefix(c.ProfilePath, "/etc/profile.d/") {
		return fmt.Errorf("connect.login_seconds, output_max_bytes, status_max_bytes, writer_bin, refresh_seconds and profile_path (/etc/profile.d) are required")
	}
	for k := range c.Env {
		if !envNameRe.MatchString(k) {
			return fmt.Errorf("connect.env.%s is not an env name", k)
		}
	}
	for id, p := range c.Providers {
		if err := p.check(id); err != nil {
			return err
		}
	}
	return nil
}

func (p ConnectProvider) check(id string) error {
	switch {
	case !provRe.MatchString(id) || len(p.Login) == 0 || len(p.Logout) == 0 || len(p.Status) == 0:
		return fmt.Errorf("connect.providers.%s needs login, logout and status argv", id)
	case !envNameRe.MatchString(p.TokenEnv) || len(p.URLPrefixes) == 0:
		return fmt.Errorf("connect.providers.%s needs token_env and url_prefixes", id)
	}
	for _, re := range []string{p.TokenPattern, p.CodePattern} {
		if _, err := regexp.Compile(re); err != nil || re == "" {
			return fmt.Errorf("connect.providers.%s: token_pattern and code_pattern must be regexps", id)
		}
	}
	for _, u := range p.URLPrefixes {
		if !strings.HasPrefix(u, "https://") {
			return fmt.Errorf("connect.providers.%s: url_prefixes must be https", id)
		}
	}
	return nil
}
