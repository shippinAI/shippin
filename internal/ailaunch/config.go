// Package ailaunch starts an AI coding CLI (Codex CLI, Claude Code, ...) in a persistent tmux
// session inside a devspace and serves it to the browser over a WebSocket. Closing the socket
// only detaches; the CLI keeps running and the next connection re-attaches to the same screen.
// People sign in with their own subscription through the CLI's normal login flow.
package ailaunch

import (
	"encoding/json"
	"errors"
	"os"
	"regexp"
)

// CLI is one launchable tool. Argv is run inside tmux as the devspace user.
type CLI struct {
	ID    string   `json:"id"`    // session suffix and URL segment, e.g. "claude"
	Label string   `json:"label"` // button text, e.g. "Claude Code"
	Argv  []string `json:"argv"`
}

// Config comes from a JSON file; nothing is hardcoded.
type Config struct {
	Addr          string   `json:"addr"`           // listen address (loopback; the edge's forward_auth fronts it)
	Origins       []string `json:"origins"`        // allowed browser origins for the WebSocket
	TmuxSocket    string   `json:"tmux_socket"`    // tmux -L name, keeps launcher sessions apart
	SessionPrefix string   `json:"session_prefix"` // e.g. "ai-"
	Static        string   `json:"static"`         // built web/dist (launcher.html)
	CLIs          []CLI    `json:"clis"`
}

var idRE = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// Load reads and checks a config file.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := &Config{}
	if err := json.Unmarshal(b, c); err != nil {
		return nil, err
	}
	if c.Addr == "" || c.TmuxSocket == "" || len(c.Origins) == 0 || len(c.CLIs) == 0 {
		return nil, errors.New("ailaunch: addr, origins, tmux_socket and clis are required")
	}
	for _, x := range c.CLIs {
		if !idRE.MatchString(x.ID) || len(x.Argv) == 0 {
			return nil, errors.New("ailaunch: every cli needs an id ([a-z0-9-]) and argv")
		}
	}
	return c, nil
}

func (c *Config) cli(id string) *CLI {
	for i := range c.CLIs {
		if c.CLIs[i].ID == id {
			return &c.CLIs[i]
		}
	}
	return nil
}
