package ssh_key

import "shippin.cloud/console/internal/mcpl"

var Delete = mcpl.Def[ByID, mcpl.None]{
	Spec: mcpl.Spec{Summary: "Remove an SSH key from your account and from your workspaces.", Access: mcpl.SignedIn},
	Run: func(c *mcpl.Call, in ByID) (mcpl.None, error) {
		tag, err := c.DB.Exec(c.Ctx, `DELETE FROM user_ssh_keys WHERE user_sub=$1 AND id::text=$2`, c.Actor, in.ID)
		if err == nil && tag.RowsAffected() == 0 {
			err = mcpl.NotFound("Key not found.")
		}
		return mcpl.None{}, err
	},
}
