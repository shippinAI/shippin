package runner

import (
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/ports"
)

// RemoveMachine is shippin.runtime.runner.remove_machine: once staff's removal is committed, it
// deletes the machine from its host straight away. If that fails the machine is left failed with the
// error, and the panel's sweep tries again.
var RemoveMachine = mcpl.Reaction{
	On:      []string{"shippin.runtime.runner.deleted"},
	Summary: "Delete the machine staff removed from its host.",
	Run: func(c *mcpl.Call, e mcpl.Event) error {
		m, err := mcpl.Need[ports.Machines](c)
		if err != nil {
			return err
		}
		_, err = Remove(c.Ctx, Deps{Pool: c.Pool, Machines: m}, e.Subject["runnerId"])
		return err
	},
}
