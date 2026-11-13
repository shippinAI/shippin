package coverage_tier

import "shippin.cloud/console/internal/mcpl"

var List = mcpl.Def[mcpl.None, mcpl.Items[Tier]]{
	Spec: mcpl.Spec{Summary: "List the coverage tiers, lowest rank first.", Access: mcpl.Staff, Perm: "catalog.plans.read"},
	Run: func(c *mcpl.Call, _ mcpl.None) (mcpl.Items[Tier], error) {
		out := mcpl.Items[Tier]{Items: []Tier{}}
		rows, err := c.DB.Query(c.Ctx, `SELECT `+cols+` FROM coverage_tiers ORDER BY rank, id`)
		if err != nil {
			return out, err
		}
		defer rows.Close()
		prices := c.Can(permPrices)
		for rows.Next() {
			t, err := scan(rows, prices)
			if err != nil {
				return out, err
			}
			out.Items = append(out.Items, t)
		}
		return out, rows.Err()
	},
}
