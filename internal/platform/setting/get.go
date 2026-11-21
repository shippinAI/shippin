package setting

import (
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/settings"
)

// Get is shippin.platform.setting.get: GET /v1/operator/platform/settings/{settingId}.
var Get = mcpl.Def[ByID, settings.View]{
	Spec: mcpl.Spec{Summary: "Read one settings section.", Access: mcpl.Staff, Perm: "platform.settings.read"},
	Run: func(c *mcpl.Call, in ByID) (settings.View, error) {
		s, err := store(c)
		if err != nil {
			return settings.View{}, err
		}
		return s.View(c.Ctx, in.ID)
	},
}
