package access_key

import "shippin.cloud/console/internal/mcpl"

var Revoke = mcpl.Def[ByID, mcpl.None]{
	Spec: mcpl.Spec{Summary: "Revoke one of your access keys; whatever uses it stops at once.", Access: mcpl.SignedIn},
	Run: func(c *mcpl.Call, in ByID) (mcpl.None, error) {
		tag, err := c.DB.Exec(c.Ctx, `DELETE FROM access_keys WHERE user_id::text = $1 AND id::text = $2`, localUser(c.Actor), in.ID)
		if err == nil && tag.RowsAffected() == 0 {
			err = mcpl.NotFound("Key not found.")
		}
		return mcpl.None{}, err
	},
}
