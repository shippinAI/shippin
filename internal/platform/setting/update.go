package setting

import (
	"encoding/json"

	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/settings"
)

// UpdateInput is a save of one section.
type UpdateInput struct {
	ByID
	// Version is the section's version the change was made from (0 before its first save); a save
	// based on an older one is refused.
	Version int `json:"version"`
	// Values are the fields to change, by name. Fields left out keep their value.
	Values map[string]json.RawMessage `json:"values,omitempty"`
	// Keys are new key values by field name; null removes the key. They go to the secret store and are
	// never returned.
	Keys map[string]*string `json:"keys,omitempty"`
	// Import moves these keys from the panel's environment into the secret store, on the section's
	// first save.
	Import []string `json:"import,omitempty"`
}

// Update is shippin.platform.setting.update: PUT /v1/operator/platform/settings/{settingId}. The whole
// section is checked before anything is saved; it takes effect without a restart.
var Update = mcpl.Def[UpdateInput, settings.View]{
	Spec: mcpl.Spec{Summary: "Change a settings section. Keys go to the secret store, never the database.", Access: mcpl.Staff, Perm: "platform.settings.write"},
	Run: func(c *mcpl.Call, in UpdateInput) (settings.View, error) {
		s, err := store(c)
		if err != nil {
			return settings.View{}, err
		}
		if len(in.Values) == 0 && len(in.Keys) == 0 && len(in.Import) == 0 {
			return settings.View{}, mcpl.Invalid("Change at least one field.")
		}
		return s.Save(c.Ctx, c.DB, settings.Change{Section: in.ID, Base: in.Version, Values: in.Values, Keys: in.Keys, Import: in.Import, By: c.Actor, Reason: c.Reason})
	},
}
