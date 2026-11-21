package acquisition_source

import (
	"time"

	"shippin.cloud/console/internal/acquisition"
	"shippin.cloud/console/internal/mcpl"
)

var Sync = mcpl.Def[ByID, SyncResult]{
	Spec: mcpl.Spec{Summary: "Import one bounded page from an authorized acquisition source.", Access: mcpl.Staff, Perm: "acquisition.sync"},
	Run: func(c *mcpl.Call, in ByID) (SyncResult, error) {
		r, err := repo(c)
		if err != nil {
			return SyncResult{}, SafeError(err)
		}
		f, err := mcpl.Need[acquisition.Fetcher](c)
		if err != nil {
			return SyncResult{}, SafeError(err)
		}
		s, err := seal(c)
		if err != nil {
			return SyncResult{}, SafeError(err)
		}
		out, err := r.Sync(c.Ctx, c.DB, in.ID, f, s, time.Now().UTC())
		return out, SafeError(err)
	},
}
