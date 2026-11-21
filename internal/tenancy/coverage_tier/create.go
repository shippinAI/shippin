package coverage_tier

import (
	"regexp"

	"shippin.cloud/console/internal/mcpl"
)

// CreateInput is a new tier: its id and its fields.
type CreateInput struct {
	ID string `json:"id"`
	Fields
}

var slug = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,39}$`)

var Create = mcpl.Def[CreateInput, Tier]{
	Spec: mcpl.Spec{Summary: "Create a coverage tier with its support hours, response targets, services and sign-in rule.", Access: mcpl.Staff, Perm: "catalog.plans.write"},
	Run: func(c *mcpl.Call, in CreateInput) (Tier, error) {
		if !slug.MatchString(in.ID) {
			return Tier{}, mcpl.Invalid("A coverage tier needs a lowercase id of 2 to 40 letters, digits, dashes or underscores.")
		}
		if err := in.normalize(); err != nil {
			return Tier{}, err
		}
		if (in.PriceCents != nil && *in.PriceCents != 0 || in.SpendPercent != nil && *in.SpendPercent != 0) && !c.Can(permPrices) {
			return Tier{}, mcpl.Forbidden("Setting a price needs catalog.prices.read.")
		}
		cents, pct := 0, 0.0
		if in.PriceCents != nil {
			cents = *in.PriceCents
		}
		if in.SpendPercent != nil {
			pct = *in.SpendPercent
		}
		_, err := c.DB.Exec(c.Ctx, `INSERT INTO coverage_tiers (id, label, description, rank, sign_in, support_hours, channels, critical_minutes,
		  high_minutes, normal_minutes, uptime_target, operations, named_contact, reviews, training_hours, consulting, custom_deployments,
		  remote_operations, team_training, price_cents, spend_percent, public, active)
		  VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)`,
			in.ID, in.Label, in.Description, in.Rank, in.SignIn, in.SupportHours, in.Channels, in.CriticalMinutes, in.HighMinutes, in.NormalMinutes,
			in.UptimeTarget, in.Operations, in.NamedContact, in.Reviews, in.TrainingHours, in.Consulting, in.CustomDeployments, in.RemoteOperations,
			in.TeamTraining, cents, pct, boolOr(in.Public), boolOr(in.Active))
		if err != nil {
			return Tier{}, err
		}
		return one(c, in.ID)
	},
}
