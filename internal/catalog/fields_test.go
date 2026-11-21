package catalog

import (
	"encoding/json"
	"testing"
)

func TestStripRemovesHiddenFields(t *testing.T) {
	raw := json.RawMessage(`[{"id":"a","price_cents":900,"label":"A"},{"id":"b","price_cents":0}]`)
	out, err := strip(raw, []string{"price_cents"})
	if err != nil || string(out) != `[{"id":"a","label":"A"},{"id":"b"}]` {
		t.Fatalf("list: %s %v", out, err)
	}
	out, err = strip(json.RawMessage(`{"id":"c","notes":"vip","contacts":[1]}`), []string{"notes", "contacts"})
	if err != nil || string(out) != `{"id":"c"}` {
		t.Fatalf("object: %s %v", out, err)
	}
	if out, _ = strip(raw, nil); string(out) != string(raw) {
		t.Fatal("nothing hidden must return the input unchanged")
	}
}
