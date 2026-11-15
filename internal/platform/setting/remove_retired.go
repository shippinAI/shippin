package setting

import "shippin.cloud/console/internal/mcpl"

// RemoveRetired is shippin.platform.setting.remove_retired: once a save is committed, it clears the
// panel's cached settings and deletes the keys the save replaced or removed from the secret store.
// One that can't be deleted stays queued, and the store's sweep tries again every minute.
var RemoveRetired = mcpl.Reaction{
	On:      []string{"shippin.platform.setting.updated"},
	Summary: "Apply a saved section at once and delete the keys it replaced from the secret store.",
	Run: func(c *mcpl.Call, _ mcpl.Event) error {
		s, err := store(c)
		if err != nil {
			return err
		}
		s.Invalidate()
		_, err = s.Sweep(c.Ctx)
		return err
	},
}
