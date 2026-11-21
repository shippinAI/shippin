package lead

import (
	"shippin.cloud/console/internal/acquisition"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/tenancy/acquisition_source"
)

type BySource = acquisition_source.ByID
type Output = acquisition.LeadListResult

var List = mcpl.Def[BySource, Output]{
	Spec: mcpl.Spec{Summary: "Review up to 100 current acquisition leads with consent details and explicit unverified status.", Access: mcpl.Staff, Perm: "acquisition.read"},
	Run: func(c *mcpl.Call, in BySource) (Output, error) {
		r, err := mcpl.Need[acquisition.Repository](c)
		if err != nil {
			return Output{}, err
		}
		s, err := mcpl.Need[acquisition.Sealer](c)
		if err != nil {
			return Output{}, err
		}
		out, err := r.ListLeads(c.Ctx, c.DB, in.ID, s)
		if err != nil {
			return Output{}, acquisition_source.SafeError(err)
		}
		for i := range out.Items {
			out.Items[i].EmailVerified = false
		}
		return out, nil
	},
}
