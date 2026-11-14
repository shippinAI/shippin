package contract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"shippin.cloud/console/internal/mcpl"
)

type Shade string

func (Shade) Values() []string { return []string{"light", "dark"} }

type Item struct {
	ID      string    `json:"id"`
	Shade   Shade     `json:"shade"`
	Note    *string   `json:"note"`
	Price   *int      `json:"price,omitempty"`
	Tags    []string  `json:"tags"`
	At      time.Time `json:"at"`
	private string
}

type ByID struct {
	ID string `path:"contractId" json:"-"`
}

type UpdateInput struct {
	ByID
	Shade Shade `json:"shade"`
	Price *int  `json:"price,omitempty"`
}

var (
	list   = mcpl.Def[struct{}, mcpl.Items[Item]]{Spec: mcpl.Spec{Summary: "List them.", Access: mcpl.Staff, Perm: "p.read"}}
	update = mcpl.Def[UpdateInput, Item]{Spec: mcpl.Spec{Summary: "Change one.", Access: mcpl.Staff, Perm: "p.write"}}
	del    = mcpl.Def[ByID, mcpl.None]{Spec: mcpl.Spec{Summary: "Remove one.", Access: mcpl.SignedIn}}
)

func TestFiles(t *testing.T) {
	_ = Item{}.private
	all := []mcpl.Registered{
		mcpl.Register("shippin.platform.contract.list", list),
		mcpl.Register("shippin.platform.contract.update", update),
		mcpl.Register("shippin.platform.contract.delete", del),
	}
	files, err := Files(all)
	if err != nil {
		t.Fatal(err)
	}
	ts := string(files["web/src/lib/api/actions.gen.ts"])
	for _, want := range []string{
		`export type ContractShade = "light" | "dark";`, // an enum, from the type's own values
		"note: string | null;",                          // a pointer is always sent, maybe null
		"price?: number | null;",                        // omitempty may be absent
		"tags: string[];",
		"at: string;",
		`"shippin.platform.contract.list": { args: [params?: Record<string, never>]; output: { items: ContractItem[] }; risk: "read"; event: never };`,
		`"shippin.platform.contract.update": { args: [params: { contractId: string }, body: ContractUpdateInput & { reason: string }]; output: ContractItem; risk: "write"; event: "shippin.platform.contract.updated" };`,
		`"shippin.platform.contract.delete": { args: [params: { contractId: string }, body?: { reason?: string }]; output: void; risk: "destructive"; event: "shippin.platform.contract.deleted" };`,
	} {
		if !strings.Contains(ts, want) {
			t.Errorf("TypeScript lacks %s\n%s", want, ts)
		}
	}
	if strings.Contains(ts, "private") || strings.Contains(ts, "ContractByID") {
		t.Errorf("TypeScript has an unexported field or a path-only body type:\n%s", ts)
	}
	var doc struct {
		Paths      map[string]map[string]map[string]any
		Components struct{ Schemas map[string]map[string]any }
	}
	if err := json.Unmarshal(files["api/openapi/actions.gen.json"], &doc); err != nil {
		t.Fatal(err)
	}
	put := doc.Paths["/operator/platform/contracts/{contractId}"]["put"]
	if put["operationId"] != "shippin.platform.contract.update" || put["requestBody"].(map[string]any)["required"] != true {
		t.Errorf("update operation: %v", put)
	}
	if _, ok := doc.Paths["/me/platform/contracts/{contractId}"]["delete"]["responses"].(map[string]any)["204"]; !ok {
		t.Error("a delete answers 204")
	}
	if e := doc.Components.Schemas["ContractShade"]["enum"]; e == nil {
		t.Errorf("enum schema: %v", doc.Components.Schemas["ContractShade"])
	}
	if req := doc.Components.Schemas["ContractItem"]["required"].([]any); len(req) != 5 {
		t.Errorf("required fields: %v (price is omitempty)", req)
	}
}

type other struct{ X int }

// Two Go types that would share a contract name are refused, not merged.
func TestNameCollision(t *testing.T) {
	m := newModel()
	m.byName["ContractItem"] = reflect.TypeFor[other]()
	m.of(reflect.TypeFor[Item]())
	if m.err == nil {
		t.Error("two types with one name were accepted")
	}
}
