package acquisition_source

import "shippin.cloud/console/internal/mcpl"

var List = mcpl.Def[mcpl.None, mcpl.Items[Summary]]{
	Spec: mcpl.Spec{Summary: "List configured acquisition sources without exposing source credentials or lead contact data.", Access: mcpl.Staff, Perm: "acquisition.read"},
	Run: func(c *mcpl.Call, _ mcpl.None) (mcpl.Items[Summary], error) {
		out := mcpl.Items[Summary]{Items: []Summary{}}
		r, err := repo(c)
		if err != nil {
			return out, SafeError(err)
		}
		out.Items, err = r.List(c.Ctx, c.DB)
		return out, SafeError(err)
	},
}
