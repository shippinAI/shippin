package coverage_tier

import (
	"strconv"

	"shippin.cloud/console/internal/mcpl"
)

var Delete = mcpl.Def[ByID, mcpl.None]{
	Spec: mcpl.Spec{Summary: "Remove a coverage tier that no plan includes; organizations moved to it fall back to their plan's.", Access: mcpl.Staff, Perm: "catalog.plans.write"},
	Run: func(c *mcpl.Call, in ByID) (mcpl.None, error) {
		var n int
		if err := c.DB.QueryRow(c.Ctx, `SELECT count(*) FROM plans WHERE coverage=$1`, in.ID).Scan(&n); err != nil {
			return mcpl.None{}, err
		}
		if n > 0 {
			return mcpl.None{}, mcpl.Conflict(strconv.Itoa(n) + " plan(s) include this coverage. Give them another tier first.")
		}
		tag, err := c.DB.Exec(c.Ctx, `DELETE FROM coverage_tiers WHERE id=$1`, in.ID)
		if err == nil && tag.RowsAffected() == 0 {
			return mcpl.None{}, mcpl.NotFound("No coverage tier " + in.ID + ".")
		}
		return mcpl.None{}, err
	},
}
