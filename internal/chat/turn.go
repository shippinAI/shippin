package chat

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"shippin.cloud/console/internal/config"
)

// Runner starts a named hostd chat command (hostclient.Client satisfies it).
type Runner interface {
	ChatRun(ctx context.Context, t, w, cmd, session, prompt string) (io.ReadCloser, error)
}

// Prompt is the instructions, the recent transcript and the new message, as one stdin prompt.
func Prompt(cfg config.Chat, history []Message, text string) string {
	var b strings.Builder
	if cfg.Instructions != "" {
		b.WriteString(cfg.Instructions + "\n\n")
	}
	if n := len(history) - cfg.HistoryTurns; n > 0 {
		history = history[n:]
	}
	if len(history) > 0 {
		b.WriteString("Conversation so far:\n")
		for _, m := range history {
			if m.Role != "error" {
				fmt.Fprintf(&b, "[%s] %s\n", m.Role, m.Content)
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("[user] " + text + "\n")
	return b.String()
}

// Result of one turn.
type Result struct {
	Text   string
	Blocks []Block
	Err    string // non-empty when the CLI failed (not signed in, missing, timed out…)
}

// Turn runs one prompt through the provider's CLI in the workspace, calling emit with each
// streamed text chunk, and returns the final text with A2UI blocks extracted.
func Turn(ctx context.Context, r Runner, cfg config.Chat, p config.ChatProvider, t, w, session, prompt string, emit func(string)) Result {
	body, err := r.ChatRun(ctx, t, w, p.Command, session, prompt)
	if err != nil {
		return Result{Err: "The workspace couldn't run " + p.Name + ": " + err.Error()}
	}
	defer body.Close()
	var acc, final strings.Builder
	var exit *int
	var stderr string
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	for sc.Scan() {
		ev, ok := ParseLine(p.Format, sc.Text())
		switch {
		case !ok:
		case ev.Exit != nil:
			exit, stderr = ev.Exit, ev.Err
		case ev.Final:
			final.WriteString(ev.Text)
		case acc.Len() < cfg.MaxReplyBytes:
			acc.WriteString(ev.Text)
			emit(ev.Text)
		}
	}
	text := acc.String()
	if final.Len() > 0 {
		text = final.String()
	}
	if exit == nil || *exit != 0 {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = strings.TrimSpace(text)
		}
		return Result{Err: fmt.Sprintf("%s exited with an error: %s", p.Name, clip(msg, 600))}
	}
	rest, blocks := Extract(text)
	return Result{Text: rest, Blocks: blocks}
}
