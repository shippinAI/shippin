package chat

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Block is one A2UI message from an agent, validated against the whitelisted catalog below
// before it is stored or rendered. Unknown types and fields are dropped, never passed through.
type Block struct {
	Type   string  `json:"type"`
	Name   string  `json:"name,omitempty"` // stable id for a server-built action (app id)
	Title  string  `json:"title,omitempty"`
	Body   string  `json:"body,omitempty"`
	Label  string  `json:"label,omitempty"`
	Prompt string  `json:"prompt,omitempty"` // a button/form sends this as the user's next message
	Href   string  `json:"href,omitempty"`   // console-relative path or https URL
	Copy   string  `json:"copy,omitempty"`   // exact text a Copy button puts on the clipboard
	Tone   string  `json:"tone,omitempty"`   // toast: success | error | info
	Fields []Field `json:"fields,omitempty"` // form
	Items  []Block `json:"items,omitempty"`  // card: buttons
}

type Field struct {
	Name    string   `json:"name"`
	Label   string   `json:"label"`
	Kind    string   `json:"kind"` // text | textarea | select
	Options []string `json:"options,omitempty"`
}

// Catalog lists the component types agents may emit.
var Catalog = []string{"card", "button", "form", "modal", "toast", "tip"}

var fence = regexp.MustCompile("(?s)```a2ui\\s*(.*?)```")
var fieldName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
var blockName = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)

// copyAllowed is the exact clipboard text a button may carry. Nil rejects every copy, so an
// agent cannot invent a shell command. internal/apps registers the official install commands.
var copyAllowed func(string) bool

// AllowCopy installs the clipboard allowlist. The function must be exact-match and side-effect free.
func AllowCopy(fn func(string) bool) { copyAllowed = fn }

// Extract pulls ```a2ui fenced JSON (an object or array) out of a reply: returns the remaining
// text and the valid blocks.
func Extract(text string) (string, []Block) {
	var out []Block
	rest := fence.ReplaceAllStringFunc(text, func(m string) string {
		raw := strings.TrimSpace(fence.FindStringSubmatch(m)[1])
		var many []Block
		if json.Unmarshal([]byte(raw), &many) != nil {
			var one Block
			if json.Unmarshal([]byte(raw), &one) != nil {
				return m // not A2UI after all: leave it visible
			}
			many = []Block{one}
		}
		for _, b := range many {
			if v, ok := Validate(b, 0); ok {
				out = append(out, v)
			}
		}
		return ""
	})
	return strings.TrimSpace(rest), out
}

// Validate keeps only catalog types with bounded text, safe links and well-formed fields.
func Validate(b Block, depth int) (Block, bool) {
	v := Block{Type: b.Type, Title: clip(b.Title, 120), Body: clip(b.Body, 2000), Label: clip(b.Label, 60), Prompt: clip(b.Prompt, 2000)}
	if blockName.MatchString(b.Name) {
		v.Name = b.Name
	}
	if safeHref(b.Href) {
		v.Href = b.Href
	}
	if copyAllowed != nil && copyAllowed(b.Copy) {
		v.Copy = b.Copy
	}
	switch b.Type {
	case "card", "modal", "tip":
		if depth == 0 {
			for _, it := range b.Items {
				if it.Type == "button" {
					if x, ok := Validate(it, 1); ok && len(v.Items) < 4 {
						v.Items = append(v.Items, x)
					}
				}
			}
		}
		return v, v.Title != "" || v.Body != ""
	case "button":
		return v, v.Label != "" && (v.Prompt != "" || v.Href != "" || v.Copy != "")
	case "toast":
		v.Tone = map[string]string{"success": "success", "error": "error"}[b.Tone]
		if v.Tone == "" {
			v.Tone = "info"
		}
		return v, v.Title != ""
	case "form":
		for _, f := range b.Fields {
			if fieldName.MatchString(f.Name) && (f.Kind == "text" || f.Kind == "textarea" || f.Kind == "select") && len(v.Fields) < 8 {
				opts := f.Options
				if len(opts) > 12 {
					opts = opts[:12]
				}
				v.Fields = append(v.Fields, Field{Name: f.Name, Label: clip(f.Label, 60), Kind: f.Kind, Options: opts})
			}
		}
		return v, len(v.Fields) > 0 && v.Prompt != ""
	}
	return Block{}, false
}

func safeHref(h string) bool {
	return (strings.HasPrefix(h, "/") && !strings.HasPrefix(h, "//")) || strings.HasPrefix(h, "https://")
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
