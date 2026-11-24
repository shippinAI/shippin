package config

import "errors"

// Chat is the console chat (overlay + /chat): each turn runs the user's OWN AI CLI inside their
// devspace through hostd (named commands only, configured in hostd.json), signed in with their
// own subscription. The console never holds a model key or a vendor credential.
type Chat struct {
	Enabled bool `json:"enabled"`
	// Providers the composer offers; Command/Check name hostd chat commands.
	Providers []ChatProvider `json:"providers"`
	// HistoryTurns is how many earlier messages are replayed into each prompt.
	HistoryTurns int `json:"history_turns"`
	// TimeoutSeconds bounds one turn; MaxReplyBytes bounds what is stored.
	TimeoutSeconds int `json:"timeout_seconds"`
	MaxReplyBytes  int `json:"max_reply_bytes"`
	// Instructions is prepended to every prompt (A2UI catalog for structured replies).
	Instructions string `json:"instructions"`
	// Tips are passive knowledge panels shown for the page the user is on.
	Tips []ChatTip `json:"tips"`
}

// ChatProvider maps a vendor CLI to hostd command names and the M2 subscription to connect.
type ChatProvider struct {
	ID           string `json:"id"`           // e.g. claude
	Name         string `json:"name"`         // e.g. Claude (your subscription)
	Command      string `json:"command"`      // hostd chat command that runs one turn (prompt on stdin)
	Check        string `json:"check"`        // hostd chat command: exit 0 = installed and signed in
	Format       string `json:"format"`       // claude-stream-json | codex-json | text
	Subscription string `json:"subscription"` // ai.providers id used by Account → AI subscriptions
}

// ChatTip is a passive tip panel for routes starting with Route.
type ChatTip struct {
	Route string `json:"route"`
	Title string `json:"title"`
	Body  string `json:"body"`
	Href  string `json:"href,omitempty"`
}

func (c Chat) Check() error {
	if !c.Enabled {
		return nil
	}
	if len(c.Providers) == 0 || c.HistoryTurns < 0 || c.TimeoutSeconds < 10 || c.MaxReplyBytes < 1024 {
		return errors.New("chat: providers, history_turns >= 0, timeout_seconds >= 10 and max_reply_bytes >= 1024 are required")
	}
	for _, p := range c.Providers {
		if p.ID == "" || p.Command == "" || p.Check == "" {
			return errors.New("chat: every provider needs id, command and check")
		}
	}
	return nil
}
