package ailaunch

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Session is what the launcher reports about one CLI's tmux session.
type Session struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Running bool   `json:"running"`
	Created int64  `json:"created,omitempty"` // unix seconds; unchanged across re-attaches
	PanePID int    `json:"pane_pid,omitempty"`
}

func (c *Config) tmux(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "tmux", append([]string{"-L", c.TmuxSocket}, args...)...)
}

func (c *Config) name(id string) string { return c.SessionPrefix + id }

// Status reads the session of cli id (Running=false when there is none).
func (c *Config) Status(ctx context.Context, x *CLI) Session {
	s := Session{ID: x.ID, Label: x.Label}
	out, err := c.tmux(ctx, "display-message", "-p", "-t", "="+c.name(x.ID)+":", "#{session_created} #{pane_pid}").Output()
	if err != nil {
		return s
	}
	f := strings.Fields(string(out))
	if len(f) == 2 {
		s.Running = true
		s.Created, _ = strconv.ParseInt(f[0], 10, 64)
		s.PanePID, _ = strconv.Atoi(f[1])
	}
	return s
}

// Ensure starts the CLI in a detached session unless it already runs (one click = idempotent).
func (c *Config) Ensure(ctx context.Context, x *CLI, rows, cols uint16) (Session, error) {
	if s := c.Status(ctx, x); s.Running {
		return s, nil
	}
	args := []string{"new-session", "-d", "-s", c.name(x.ID), "-x", strconv.Itoa(int(cols)), "-y", strconv.Itoa(int(rows)), "--"}
	if err := c.tmux(ctx, append(args, x.Argv...)...).Run(); err != nil {
		return Session{}, err
	}
	// keep the window when the CLI exits so the person sees why; status bar off, mouse on
	_ = c.tmux(ctx, "set-option", "-t", c.name(x.ID), "remain-on-exit", "on").Run()
	_ = c.tmux(ctx, "set-option", "-t", c.name(x.ID), "status", "off").Run()
	_ = c.tmux(ctx, "set-option", "-t", c.name(x.ID), "mouse", "on").Run()
	time.Sleep(150 * time.Millisecond)
	return c.Status(ctx, x), nil
}

// Stop ends the CLI's session.
func (c *Config) Stop(ctx context.Context, x *CLI) error {
	return c.tmux(ctx, "kill-session", "-t", "="+c.name(x.ID)).Run()
}

// attachCmd is the per-connection client; killing it only detaches.
func (c *Config) attachCmd(x *CLI) *exec.Cmd {
	return exec.Command("tmux", "-L", c.TmuxSocket, "attach-session", "-t", "="+c.name(x.ID))
}
