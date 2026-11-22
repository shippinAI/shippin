package version

import (
	"encoding/json"
	"time"

	"shippin.cloud/console/internal/mcpl"
)

// Version is one saved version of a settings section: who saved it, why, its values and which keys
// changed. Key values are never part of it.
type Version struct {
	Version     int             `json:"version"`
	Values      json.RawMessage `json:"values"`
	KeysChanged []string        `json:"keys_changed"`
	ChangedBy   string          `json:"changed_by"`
	Reason      string          `json:"reason"`
	ChangedAt   time.Time       `json:"changed_at"`
}

// ListInput names the section.
type ListInput struct {
	Setting string `path:"settingId" json:"-"`
}

// List is shippin.platform.setting.version.list: GET /v1/operator/platform/settings/{settingId}/versions,
// the newest 100 first.
var List = mcpl.Def[ListInput, mcpl.Items[Version]]{
	Spec: mcpl.Spec{Summary: "See who changed a settings section, when and why.", Access: mcpl.Staff, Perm: "platform.settings.read"},
	Run: func(c *mcpl.Call, in ListInput) (mcpl.Items[Version], error) {
		out := mcpl.Items[Version]{Items: []Version{}}
		rows, err := c.DB.Query(c.Ctx, `SELECT version, vals, keys_changed, changed_by, reason, changed_at FROM shippin_setting_versions
		  WHERE section=$1 ORDER BY version DESC LIMIT 100`, in.Setting)
		if err != nil {
			return out, err
		}
		defer rows.Close()
		for rows.Next() {
			var v Version
			if err := rows.Scan(&v.Version, &v.Values, &v.KeysChanged, &v.ChangedBy, &v.Reason, &v.ChangedAt); err != nil {
				return out, err
			}
			out.Items = append(out.Items, v)
		}
		return out, rows.Err()
	},
}
