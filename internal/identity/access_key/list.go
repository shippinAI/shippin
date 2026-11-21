package access_key

import "shippin.cloud/console/internal/mcpl"

var List = mcpl.Def[struct{}, mcpl.Items[Key]]{
	Spec: mcpl.Spec{Summary: "List your access keys, with when each expires and was last used.", Access: mcpl.SignedIn},
	Run: func(c *mcpl.Call, _ struct{}) (mcpl.Items[Key], error) {
		rows, err := c.DB.Query(c.Ctx, `SELECT id::text, name, prefix, created_at, expires_at, last_used_at FROM access_keys
			WHERE user_id::text = $1 ORDER BY created_at`, localUser(c.Actor))
		if err != nil {
			return mcpl.Items[Key]{}, err
		}
		defer rows.Close()
		out := []Key{}
		for rows.Next() {
			var k Key
			if err := rows.Scan(&k.ID, &k.Name, &k.Prefix, &k.CreatedAt, &k.ExpiresAt, &k.LastUsedAt); err != nil {
				return mcpl.Items[Key]{}, err
			}
			out = append(out, k)
		}
		return mcpl.Items[Key]{Items: out}, rows.Err()
	},
}
