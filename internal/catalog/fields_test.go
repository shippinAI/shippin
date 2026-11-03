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

func TestHiddenFieldsAreKeptAndNeverSet(t *testing.T) {
	stored := Customer{Contacts: json.RawMessage(`[{"name":"Ann"}]`), Endpoints: json.RawMessage(`[{"host":"x"}]`), Notes: "vip"}
	in := Customer{Name: "New", Contacts: json.RawMessage(`[]`), Endpoints: json.RawMessage(`[]`)}
	keepHidden(&in, stored, []string{"contacts", "endpoints", "notes"})
	if string(in.Contacts) != `[{"name":"Ann"}]` || string(in.Endpoints) != `[{"host":"x"}]` || in.Notes != "vip" || in.Name != "New" {
		t.Fatalf("hidden fields not kept: %+v", in)
	}
	if setsHidden(Customer{Contacts: json.RawMessage(`[]`)}, []string{"contacts", "notes"}) {
		t.Fatal("empty values are not a write")
	}
	if !setsHidden(Customer{Notes: "x"}, []string{"notes"}) || !setsHidden(Customer{Endpoints: json.RawMessage(`[1]`)}, []string{"endpoints"}) {
		t.Fatal("a value in a hidden field must be refused")
	}
}
