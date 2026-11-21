package catalog

import "encoding/json"

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
