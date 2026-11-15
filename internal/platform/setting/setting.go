// Package setting is the panel's settings sections (CFG-01, docs/settings.md). The sections themselves
// are declared by their owners in settings.go files and collected into actions.Settings; this entity
// reads and saves them through the installation's *settings.Store.
package setting

import (
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/settings"
)

// ByID names one section from the route.
type ByID struct {
	ID string `path:"settingId" json:"-"`
}

func store(c *mcpl.Call) (*settings.Store, error) { return mcpl.Need[*settings.Store](c) }
