// Package catalog holds plans, the service catalog, layered tenant defaults and the merge that
// turns them into one effective configuration per tenant.
package catalog

import (
	"encoding/json"
	"sort"
)

// Lock modes (doc 35): what a child layer may do with a value defined above it.
const (
	LockLocked      = "locked"
	LockOverridable = "overridable"
	LockMergeable   = "mergeable"
	LockBounded     = "bounded"
)

// LayerOrder is the precedence chain, parent first.
var LayerOrder = []string{"system", "plan", "tenant", "workspace"}

// Item is one setting defined at one layer.
type Item struct {
	Layer  string          `json:"layer"`
	Ref    string          `json:"ref"`
	Key    string          `json:"key"`
	Value  json.RawMessage `json:"value"`
	Lock   string          `json:"lock"`
	Bounds *Bounds         `json:"bounds,omitempty"`
	Note   string          `json:"note,omitempty"`
}

// Bounds limit what a child may set under the bounded lock.
type Bounds struct {
	Min     *float64          `json:"min,omitempty"`
	Max     *float64          `json:"max,omitempty"`
	Allowed []json.RawMessage `json:"allowed,omitempty"`
}

// Step records what one layer tried and what happened.
type Step struct {
	Layer   string          `json:"layer"`
	Ref     string          `json:"ref"`
	Value   json.RawMessage `json:"value"`
	Outcome string          `json:"outcome"` // set, replaced, merged, clamped, rejected
	Reason  string          `json:"reason,omitempty"`
}

// Effective is the merged value of one key and where it came from.
type Effective struct {
	Key    string          `json:"key"`
	Value  json.RawMessage `json:"value"`
	Source string          `json:"source"` // layer that last changed the value
	Ref    string          `json:"ref"`
	Lock   string          `json:"lock"`
	Bounds *Bounds         `json:"bounds,omitempty"`
	Note   string          `json:"note,omitempty"`
	Flags  []string        `json:"flags"`
	Trail  []Step          `json:"trail"`
}

func layerRank(l string) int {
	for i, x := range LayerOrder {
		if x == l {
			return i
		}
	}
	return len(LayerOrder)
}

// Merge applies items parent-first per key and returns effective values sorted by key.
func Merge(items []Item) []Effective {
	sorted := append([]Item(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool { return layerRank(sorted[i].Layer) < layerRank(sorted[j].Layer) })
	byKey := map[string]*Effective{}
	for _, it := range sorted {
		e, ok := byKey[it.Key]
		if !ok {
			byKey[it.Key] = &Effective{Key: it.Key, Value: it.Value, Source: it.Layer, Ref: it.Ref, Lock: lockOr(it.Lock),
				Bounds: it.Bounds, Note: it.Note, Flags: []string{}, Trail: []Step{{it.Layer, it.Ref, it.Value, "set", ""}}}
			continue
		}
		apply(e, it)
	}
	out := make([]Effective, 0, len(byKey))
	for _, e := range byKey {
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

func lockOr(l string) string {
	if l == "" {
		return LockOverridable
	}
	return l
}

func apply(e *Effective, it Item) {
	step := Step{Layer: it.Layer, Ref: it.Ref, Value: it.Value}
	switch e.Lock {
	case LockLocked:
		step.Outcome, step.Reason = "rejected", "locked by "+e.Source
	case LockMergeable:
		e.Value = mergeLists(e.Value, it.Value)
		step.Outcome = "merged"
		e.Source, e.Ref = it.Layer, it.Ref
	case LockBounded:
		v, clamped, reason := clamp(it.Value, e.Bounds)
		e.Value, e.Source, e.Ref = v, it.Layer, it.Ref
		step.Outcome = "replaced"
		if clamped {
			step.Outcome, step.Reason = "clamped", reason
			e.Flags = append(e.Flags, "clamped at "+it.Layer+": "+reason)
		}
		if it.Bounds != nil {
			e.Bounds = narrow(e.Bounds, it.Bounds)
		}
	default:
		e.Value, e.Source, e.Ref, e.Lock = it.Value, it.Layer, it.Ref, lockOr(it.Lock)
		if it.Bounds != nil {
			e.Bounds = it.Bounds
		}
		step.Outcome = "replaced"
	}
	if it.Note != "" && step.Outcome != "rejected" {
		e.Note = it.Note
	}
	e.Trail = append(e.Trail, step)
}
