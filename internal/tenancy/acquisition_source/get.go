package acquisition_source

import "shippin.cloud/console/internal/mcpl"

var Get = mcpl.Def[ByID, Detail]{
	Spec: mcpl.Spec{Summary: "Read a source's nonsecret connection and trust configuration.", Access: mcpl.Staff, Perm: "platform.settings.write"},
	Run: func(c *mcpl.Call, in ByID) (Detail, error) {
		r, err := repo(c)
		if err != nil {
			return Detail{}, SafeError(err)
		}
		out, err := r.Get(c.Ctx, c.DB, in.ID)
		return out, SafeError(err)
	},
}
