package catalog

import (
	"encoding/json"
	"net/http"
)

// can is the field-level check for the current caller (nil Can = loopback demo: allow).
func (h *Handler) can(r *http.Request, perm string) bool { return h.Can == nil || h.Can(r, perm) }

// hiddenPlanFields / hiddenCustomerFields name the JSON keys a caller may not see.
func (h *Handler) hiddenPlanFields(r *http.Request) []string {
	if h.can(r, PermPricesRead) {
		return nil
	}
	return []string{"price_cents"}
}

func (h *Handler) hiddenCustomerFields(r *http.Request) []string {
	var out []string
	if !h.can(r, PermContactsRead) {
		out = append(out, "contacts", "endpoints")
	}
	if !h.can(r, PermNotesRead) {
		out = append(out, "notes")
	}
	return out
}

// strip removes keys from a JSON object, or from every object of a JSON array. The server
// omits the field entirely (not null, not masked), so a missing permission can't leak a value.
func strip(raw json.RawMessage, keys []string) (json.RawMessage, error) {
	if len(keys) == 0 || len(raw) == 0 {
		return raw, nil
	}
	var list []map[string]json.RawMessage
	if raw[0] == '[' {
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, err
		}
	} else {
		var one map[string]json.RawMessage
		if err := json.Unmarshal(raw, &one); err != nil {
			return nil, err
		}
		list = []map[string]json.RawMessage{one}
	}
	for _, m := range list {
		for _, k := range keys {
			delete(m, k)
		}
	}
	if raw[0] == '[' {
		return json.Marshal(list)
	}
	return json.Marshal(list[0])
}

// keepHidden copies the stored values of fields the caller can't see into an update, so a
// write without customers.contacts.read / customers.notes.read never blanks them.
func keepHidden(in *Customer, stored Customer, hidden []string) {
	for _, k := range hidden {
		switch k {
		case "contacts":
			in.Contacts = stored.Contacts
		case "endpoints":
			in.Endpoints = stored.Endpoints
		case "notes":
			in.Notes = stored.Notes
		}
	}
}

// setsHidden reports whether a new customer carries values in fields the caller can't see.
func setsHidden(c Customer, hidden []string) bool {
	for _, k := range hidden {
		switch k {
		case "contacts":
			if !emptyList(c.Contacts) {
				return true
			}
		case "endpoints":
			if !emptyList(c.Endpoints) {
				return true
			}
		case "notes":
			if c.Notes != "" {
				return true
			}
		}
	}
	return false
}

func emptyList(raw json.RawMessage) bool {
	return len(raw) == 0 || string(raw) == "[]" || string(raw) == "null"
}
