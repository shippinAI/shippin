package catalog

import (
	"encoding/json"
	"testing"
)

func f(v float64) *float64 { return &v }

func get(t *testing.T, es []Effective, key string) Effective {
	t.Helper()
	for _, e := range es {
		if e.Key == key {
			return e
		}
	}
	t.Fatalf("key %s missing", key)
	return Effective{}
}

func raw(s string) json.RawMessage { return json.RawMessage(s) }

func TestLockedRejectsChildren(t *testing.T) {
	e := get(t, Merge([]Item{
		{Layer: "tenant", Ref: "t1", Key: "k", Value: raw(`"off"`)},
		{Layer: "system", Key: "k", Value: raw(`"required"`), Lock: LockLocked},
	}), "k")
	if string(e.Value) != `"required"` || e.Source != "system" || e.Trail[1].Outcome != "rejected" {
		t.Fatalf("locked value changed: %+v", e)
	}
}

func TestOverridableChildWinsAndCanTighten(t *testing.T) {
	es := Merge([]Item{
		{Layer: "system", Key: "k", Value: raw(`"direct"`), Lock: LockOverridable},
		{Layer: "plan", Ref: "pro", Key: "k", Value: raw(`"litellm"`), Lock: LockLocked},
		{Layer: "tenant", Ref: "t1", Key: "k", Value: raw(`"direct"`)},
	})
	e := get(t, es, "k")
	if string(e.Value) != `"litellm"` || e.Source != "plan" || e.Ref != "pro" || e.Lock != LockLocked {
		t.Fatalf("got %+v", e)
	}
}

func TestMergeableUnionDenyBeatsAllow(t *testing.T) {
	e := get(t, Merge([]Item{
		{Layer: "system", Key: "egress", Value: raw(`["github.com","!evil.com"]`), Lock: LockMergeable},
		{Layer: "plan", Ref: "team", Key: "egress", Value: raw(`["pypi.org"]`)},
		{Layer: "tenant", Ref: "t1", Key: "egress", Value: raw(`["evil.com","pypi.org"]`)},
	}), "egress")
	var got []string
	_ = json.Unmarshal(e.Value, &got)
	want := []string{"github.com", "!evil.com", "pypi.org"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	if e.Source != "tenant" || e.Trail[2].Outcome != "merged" {
		t.Fatalf("source %s", e.Source)
	}
}

func TestBoundedClampsAndFlags(t *testing.T) {
	e := get(t, Merge([]Item{
		{Layer: "system", Key: "idle", Value: raw(`60`), Lock: LockBounded, Bounds: &Bounds{Min: f(15), Max: f(480)}},
		{Layer: "plan", Ref: "free", Key: "idle", Value: raw(`30`), Bounds: &Bounds{Max: f(60)}},
		{Layer: "tenant", Ref: "t1", Key: "idle", Value: raw(`120`)},
	}), "idle")
	if string(e.Value) != "60" || e.Trail[2].Outcome != "clamped" || len(e.Flags) != 1 {
		t.Fatalf("want clamp to plan max 60, got %+v", e)
	}
	if *e.Bounds.Max != 60 || *e.Bounds.Min != 15 {
		t.Fatalf("bounds not narrowed: %+v", e.Bounds)
	}
}

func TestBoundedChildCannotWiden(t *testing.T) {
	e := get(t, Merge([]Item{
		{Layer: "system", Key: "n", Value: raw(`5`), Lock: LockBounded, Bounds: &Bounds{Max: f(10)}},
		{Layer: "plan", Ref: "p", Key: "n", Value: raw(`8`), Bounds: &Bounds{Max: f(100)}},
		{Layer: "tenant", Ref: "t", Key: "n", Value: raw(`50`)},
	}), "n")
	if string(e.Value) != "10" {
		t.Fatalf("widened bound leaked: %s", e.Value)
	}
}

func TestBoundedAllowedSet(t *testing.T) {
	e := get(t, Merge([]Item{
		{Layer: "system", Key: "size", Value: raw(`"s"`), Lock: LockBounded, Bounds: &Bounds{Allowed: []json.RawMessage{raw(`"s"`), raw(`"m"`)}}},
		{Layer: "tenant", Ref: "t", Key: "size", Value: raw(`"xl"`)},
	}), "size")
	if string(e.Value) != `"s"` || len(e.Flags) != 1 {
		t.Fatalf("got %+v", e)
	}
}

func TestLayerOrderIndependentOfInput(t *testing.T) {
	e := get(t, Merge([]Item{
		{Layer: "workspace", Ref: "w", Key: "k", Value: raw(`3`)},
		{Layer: "tenant", Ref: "t", Key: "k", Value: raw(`2`)},
		{Layer: "system", Key: "k", Value: raw(`1`)},
	}), "k")
	if string(e.Value) != "3" || e.Source != "workspace" || len(e.Trail) != 3 || e.Trail[0].Layer != "system" {
		t.Fatalf("got %+v", e)
	}
}
