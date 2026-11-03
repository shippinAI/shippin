package hostd

import (
	"fmt"
	"regexp"
	"strings"
)

// AgentConfig (M4) runs a build agent inside a user's workspace with the user's OWN signed-in AI
// CLI (their login is on their AI home). Shippin never holds or routes a token: hostd only writes
// the prompt file and starts the CLI in tmux. Empty session disables every agent route.
type AgentConfig struct {
	// User/UID run the agent and own the prompt file (the image's dev user).
	User string `json:"user"`
	UID  int    `json:"uid"`
	// Session is the tmux session name the agent and its terminal share.
	Session string `json:"session"`
	// SessionManager boots that tmux server with one window per provider (phase 1). Default
	// false: a workspace keeps today's single on-demand session. See docs/design/devspace-vm.md.
	SessionManager bool `json:"session_manager"`
	// PromptMaxBytes caps the prompt file (the console caps it too).
	PromptMaxBytes int `json:"prompt_max_bytes"`
	// TermPort/TermIface/TermBase: ttyd attached to the session, published at /_term/ on the
	// workspace host behind the same sign-in as the IDE. TermPort 0 = no browser terminal.
	TermPort  int    `json:"term_port"`
	TermIface string `json:"term_iface"`
	TermBase  string `json:"term_base"`
	// Providers maps a provider id to the CLI argv for a new run (the prompt is appended by the
	// in-workspace wrapper, never by hostd) and for resuming the last conversation.
	Providers map[string]AgentProvider `json:"providers"`
}

// AgentProvider is one vendor CLI's argv. No flag here may skip the CLI's own permission prompts.
type AgentProvider struct {
	Run    []string `json:"run"`
	Resume []string `json:"resume"`
}

var (
	sessionRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
	userRe    = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	argRe     = regexp.MustCompile(`^[A-Za-z0-9-][A-Za-z0-9._=/:-]{0,63}$`)
	baseRe    = regexp.MustCompile(`^/_[a-z]{2,16}$`)
)

func (a AgentConfig) enabled() bool { return a.Session != "" }

// sessions is the phase-1 session manager: one tmux server, one window per provider, started
// when the workspace boots. Off unless agent.session_manager is set.
func (a AgentConfig) sessions() bool { return a.enabled() && a.SessionManager }

// check fails closed on an enabled but incomplete or unsafe section.
func (a AgentConfig) check() error {
	if a.SessionManager && !a.enabled() {
		return fmt.Errorf("agent.session_manager requires agent.session")
	}
	if !a.enabled() {
		return nil
	}
	switch {
	case !sessionRe.MatchString(a.Session) || !userRe.MatchString(a.User) || a.UID < 1:
		return fmt.Errorf("agent.session, user and uid are required")
	case a.PromptMaxBytes < 256 || a.PromptMaxBytes > 64<<10:
		return fmt.Errorf("agent.prompt_max_bytes must be 256-65536")
	case a.TermPort != 0 && (a.TermPort < 1024 || a.TermPort > 65535 || !ifaceRe.MatchString(a.TermIface) || !baseRe.MatchString(a.TermBase)):
		return fmt.Errorf("agent.term_port needs term_iface and term_base (/_name)")
	case len(a.Providers) == 0:
		return fmt.Errorf("agent.providers must list at least one CLI")
	}
	for p, v := range a.Providers {
		if !provRe.MatchString(p) || len(v.Run) == 0 || len(v.Resume) == 0 {
			return fmt.Errorf("agent.providers.%s needs run and resume argv", p)
		}
		for _, arg := range append(append([]string{}, v.Run...), v.Resume...) {
			if !argRe.MatchString(arg) || blockedAgentArg(arg) {
				return fmt.Errorf("agent.providers.%s: argument %q not allowed", p, arg)
			}
		}
	}
	return nil
}

// blockedAgentArg is a vendor flag that skips the CLI's own permission prompts. Grok's
// --always-approve is an alias of --yolo (xAI CLI reference).
func blockedAgentArg(arg string) bool {
	switch arg {
	case "--dangerously-skip-permissions", "--dangerously-bypass-approvals-and-sandbox",
		"--yolo", "--always-approve":
		return true
	default:
		return strings.Contains(strings.ToLower(arg), "bypasspermissions")
	}
}
