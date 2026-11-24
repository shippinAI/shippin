package chat

import (
	"encoding/json"
	"strings"
)

// Event is one parsed piece of CLI output.
type Event struct {
	Text  string // assistant text to append
	Final bool   // the CLI's own final result (replaces the streamed text when non-empty)
	Exit  *int   // hostd trailer: CLI exit code
	Err   string // hostd trailer stderr
}

// ParseLine turns one output line into an event for the given format. Lines that are not the
// expected JSON are treated as plain text for format "text" and ignored otherwise.
func ParseLine(format, line string) (Event, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return Event{}, false
	}
	var m map[string]any
	if json.Unmarshal([]byte(line), &m) != nil {
		if format == "text" {
			return Event{Text: line + "\n"}, true
		}
		return Event{}, false
	}
	if c, ok := m["shippin_exit"].(float64); ok {
		code := int(c)
		e, _ := m["stderr"].(string)
		return Event{Exit: &code, Err: e}, true
	}
	switch format {
	case "claude-stream-json":
		if m["type"] == "assistant" {
			msg, _ := m["message"].(map[string]any)
			content, _ := msg["content"].([]any)
			var b strings.Builder
			for _, c := range content {
				if cm, _ := c.(map[string]any); cm["type"] == "text" {
					s, _ := cm["text"].(string)
					b.WriteString(s)
				}
			}
			return Event{Text: b.String()}, b.Len() > 0
		}
		if m["type"] == "result" {
			s, _ := m["result"].(string)
			return Event{Text: s, Final: true}, true
		}
	case "codex-json":
		if item, _ := m["item"].(map[string]any); m["type"] == "item.completed" && item["type"] == "agent_message" {
			s, _ := item["text"].(string)
			return Event{Text: s + "\n"}, s != ""
		}
	}
	return Event{}, false
}
