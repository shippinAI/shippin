package setting

import (
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/settings"
)

// List is shippin.platform.setting.list: GET /v1/operator/platform/settings, every section by title
// with its values in force and where each comes from.
var List = mcpl.Def[mcpl.None, mcpl.Items[settings.View]]{
	Spec: mcpl.Spec{Summary: "List the installation's settings sections, with each value and where it comes from.", Access: mcpl.Staff, Perm: "platform.settings.read"},
	Run: func(c *mcpl.Call, _ mcpl.None) (mcpl.Items[settings.View], error) {
		out := mcpl.Items[settings.View]{Items: []settings.View{}}
		s, err := store(c)
		if err != nil {
			return out, err
		}
		for _, sec := range s.Sections() {
			v, err := s.View(c.Ctx, sec.Key)
			if err != nil {
				return out, err
			}
			out.Items = append(out.Items, v)
		}
		return out, nil
	},
}
