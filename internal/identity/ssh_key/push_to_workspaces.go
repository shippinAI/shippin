package ssh_key

import (
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/ports"
)

// PushToWorkspaces gives a person's workspaces their current keys whenever they add or remove one,
// so a removed key stops working on running workspaces too.
var PushToWorkspaces = mcpl.Reaction{
	On:      []string{"shippin.identity.ssh_key.created", "shippin.identity.ssh_key.deleted"},
	Summary: "Give the person's workspaces their current SSH keys.",
	Run: func(c *mcpl.Call, e mcpl.Event) error {
		w, err := mcpl.Need[ports.Workspaces](c)
		if err != nil {
			return err
		}
		keys, err := PublicKeys(c.Ctx, c.Pool, e.Actor)
		if err != nil {
			return err
		}
		return w.GiveKeys(c.Ctx, e.Actor, keys)
	},
}
