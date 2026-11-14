package ssh_key

import "shippin.cloud/console/internal/mcpl"

var List = mcpl.Def[struct{}, mcpl.Items[Key]]{
	Spec: mcpl.Spec{Summary: "List the SSH keys on your account, which Shippin puts on the workspaces you create.", Access: mcpl.SignedIn},
	Run: func(c *mcpl.Call, _ struct{}) (mcpl.Items[Key], error) {
		ks, err := Of(c.Ctx, c.DB, c.Actor)
		return mcpl.Items[Key]{Items: ks}, err
	},
}
