package catalog

import (
	"encoding/json"
	"fmt"
	"strings"
)

// mergeLists unions two string lists; entries starting with "!" are denies and deny beats
// allow, so a child cannot re-allow what a parent denied. Non-list values: child is ignored.
func mergeLists(parent, child json.RawMessage) json.RawMessage {
	var p, c []string
	if json.Unmarshal(parent, &p) != nil || json.Unmarshal(child, &c) != nil {
		return parent
	}
	deny := map[string]bool{}
	for _, s := range append(append([]string{}, p...), c...) {
		if strings.HasPrefix(s, "!") {
			deny[s[1:]] = true
		}
	}
	seen, out := map[string]bool{}, []string{}
	for _, s := range append(p, c...) {
		if seen[s] || (!strings.HasPrefix(s, "!") && deny[s]) {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	b, _ := json.Marshal(out)
	return b
}

// clamp keeps a number inside min/max, or a value inside allowed (falls back to the first).
func clamp(v json.RawMessage, b *Bounds) (json.RawMessage, bool, string) {
	if b == nil {
		return v, false, ""
	}
	if len(b.Allowed) > 0 {
		for _, a := range b.Allowed {
			if string(compact(a)) == string(compact(v)) {
				return v, false, ""
			}
		}
		return b.Allowed[0], true, "not in allowed set"
	}
	var n float64
	if json.Unmarshal(v, &n) != nil {
		return v, false, ""
	}
	if b.Max != nil && n > *b.Max {
		return num(*b.Max), true, fmt.Sprintf("above max %v", *b.Max)
	}
	if b.Min != nil && n < *b.Min {
		return num(*b.Min), true, fmt.Sprintf("below min %v", *b.Min)
	}
	return v, false, ""
}

// narrow lets a child tighten (never widen) the parent's bounds.
func narrow(p, c *Bounds) *Bounds {
	if p == nil {
		return c
	}
	out := *p
	if c.Max != nil && (p.Max == nil || *c.Max < *p.Max) {
		out.Max = c.Max
	}
	if c.Min != nil && (p.Min == nil || *c.Min > *p.Min) {
		out.Min = c.Min
	}
	return &out
}

func num(f float64) json.RawMessage { b, _ := json.Marshal(f); return b }

func compact(b json.RawMessage) []byte {
	var v any
	if json.Unmarshal(b, &v) != nil {
		return b
	}
	out, _ := json.Marshal(v)
	return out
}
