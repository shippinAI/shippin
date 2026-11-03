package catalog

import (
	"encoding/json"
	"sort"
)

// svcKey is the effective-config key for a service setting.
func svcKey(svc, k string) string { return "svc." + svc + "." + k }

// ServiceItems turns the catalog, the plan's matrix row and tenant overrides into layered items,
// so services merge with the same rules as every other setting:
//   - system: enabled = required (locked when required), each default_config key overridable
//   - plan: enabled per matrix (locked off when the plan doesn't include it); numeric limits
//     become the value and, under "bounded", the max a tenant may set
//   - tenant: tenant_services enabled and overrides
func ServiceItems(plan string, svcs []Service, cells []PlanService, ts []TenantService) []Item {
	out := []Item{}
	cellBy := map[string]PlanService{}
	for _, c := range cells {
		if c.PlanID == plan {
			cellBy[c.ServiceID] = c
		}
	}
	for _, s := range svcs {
		if !s.Active {
			continue
		}
		lock := LockOverridable
		if s.Required {
			lock = LockLocked
		}
		out = append(out, Item{Layer: "system", Key: svcKey(s.ID, "enabled"), Value: boolJSON(s.Required), Lock: lock,
			Note: s.Label + (map[bool]string{true: " is required on every plan", false: " is optional"})[s.Required]})
		for _, k := range sortedKeys(s.DefaultConfig) {
			out = append(out, Item{Layer: "system", Key: svcKey(s.ID, k), Value: field(s.DefaultConfig, k), Lock: LockOverridable})
		}
		c, ok := cellBy[s.ID]
		if !ok {
			out = append(out, Item{Layer: "plan", Ref: plan, Key: svcKey(s.ID, "enabled"), Value: boolJSON(false), Lock: LockLocked,
				Note: "Not included in plan " + plan})
			continue
		}
		enLock := LockOverridable
		if c.Lock == LockLocked {
			enLock = LockLocked
		}
		out = append(out, Item{Layer: "plan", Ref: plan, Key: svcKey(s.ID, "enabled"), Value: boolJSON(c.Enabled), Lock: enLock})
		for _, k := range sortedKeys(c.Limits) {
			it := Item{Layer: "plan", Ref: plan, Key: svcKey(s.ID, k), Value: field(c.Limits, k), Lock: c.Lock}
			var n float64
			if c.Lock == LockBounded && json.Unmarshal(it.Value, &n) == nil {
				zero := 0.0
				it.Bounds = &Bounds{Min: &zero, Max: &n}
			}
			out = append(out, it)
		}
	}
	for _, t := range ts {
		if t.Enabled != nil {
			out = append(out, Item{Layer: "tenant", Ref: t.TenantID, Key: svcKey(t.ServiceID, "enabled"), Value: boolJSON(*t.Enabled)})
		}
		for _, k := range sortedKeys(t.Overrides) {
			out = append(out, Item{Layer: "tenant", Ref: t.TenantID, Key: svcKey(t.ServiceID, k), Value: field(t.Overrides, k)})
		}
	}
	return out
}

func boolJSON(b bool) json.RawMessage { v, _ := json.Marshal(b); return v }

func sortedKeys(raw json.RawMessage) []string {
	m := map[string]json.RawMessage{}
	_ = json.Unmarshal(raw, &m)
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func field(raw json.RawMessage, k string) json.RawMessage {
	m := map[string]json.RawMessage{}
	_ = json.Unmarshal(raw, &m)
	return m[k]
}
