package coverage_tier

import "shippin.cloud/console/internal/mcpl"

// UpdateInput is a tier's new fields. A caller without catalog.prices.read keeps the stored price.
type UpdateInput struct {
	ByID
	Fields
}

var Update = mcpl.Def[UpdateInput, Tier]{
	Spec: mcpl.Spec{Summary: "Change a coverage tier's support, services, sign-in rule or price.", Access: mcpl.Staff, Perm: "catalog.plans.write"},
	Run: func(c *mcpl.Call, in UpdateInput) (Tier, error) {
		if err := in.normalize(); err != nil {
			return Tier{}, err
		}
		price := `price_cents=coalesce($20, price_cents), spend_percent=coalesce($21, spend_percent)`
		if !c.Can(permPrices) {
			in.PriceCents, in.SpendPercent = nil, nil
		}
		tag, err := c.DB.Exec(c.Ctx, `UPDATE coverage_tiers SET label=$2, description=$3, rank=$4, sign_in=$5, support_hours=$6, channels=$7,
		  critical_minutes=$8, high_minutes=$9, normal_minutes=$10, uptime_target=$11, operations=$12, named_contact=$13, reviews=$14,
		  training_hours=$15, consulting=$16, custom_deployments=$17, remote_operations=$18, team_training=$19, `+price+`,
		  public=$22, active=$23, updated_at=now() WHERE id=$1`,
			in.ID, in.Label, in.Description, in.Rank, in.SignIn, in.SupportHours, in.Channels, in.CriticalMinutes, in.HighMinutes, in.NormalMinutes,
			in.UptimeTarget, in.Operations, in.NamedContact, in.Reviews, in.TrainingHours, in.Consulting, in.CustomDeployments, in.RemoteOperations,
			in.TeamTraining, in.PriceCents, in.SpendPercent, boolOr(in.Public), boolOr(in.Active))
		if err != nil {
			return Tier{}, err
		}
		if tag.RowsAffected() == 0 {
			return Tier{}, mcpl.NotFound("No coverage tier " + in.ID + ".")
		}
		return one(c, in.ID)
	},
}
