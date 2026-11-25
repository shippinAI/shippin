package devspace

import "shippin.cloud/console/internal/mcpl"

// WakeWorkers starts the accepted work at once instead of at the workers' next look. It runs after
// the action is committed, so the work is there to find.
var WakeWorkers = mcpl.Reaction{
	On: []string{"shippin.runtime.devspace.started", "shippin.runtime.devspace.stopped",
		"shippin.runtime.devspace.restarted", "shippin.runtime.devspace.deleted"},
	Summary: "Tell the workers there is work on a development space.",
	Run: func(c *mcpl.Call, _ mcpl.Event) error {
		q, err := mcpl.Need[Queue](c)
		if err != nil {
			return err
		}
		q.Wake()
		return nil
	},
}
