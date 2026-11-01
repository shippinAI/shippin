package api

import (
	"context"
	"log"
	"slices"
	"time"
)

// SyncStaff seeds the preset roles on a fresh database and creates any role missing from the
// Nomen project (best effort; logged). Nomen roles not in the table are left alone.
func (a *API) SyncStaff(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := a.db.SeedStaff(ctx); err != nil {
		log.Printf("staff seed: %v", err)
	}
	if a.cfg.Staff.TeamToken == "" {
		log.Printf("staff: SHIPPIN_IDP_TEAM_TOKEN not set; staff roles grant nothing until it is")
		return
	}
	roles, err := a.db.StaffRoles(ctx)
	if err != nil {
		log.Printf("staff sync: %v", err)
		return
	}
	have, err := a.idp().RoleKeys(ctx)
	if err != nil {
		log.Printf("staff sync: project roles: %v", err)
		return
	}
	for _, r := range roles {
		if slices.Contains(have, r.Key) {
			continue
		}
		if err := a.idp().PutRole(ctx, r.Key, r.Name, false); err != nil {
			log.Printf("staff sync %s: %v", r.Key, err)
		}
	}
	log.Printf("staff: %d roles checked against project %s", len(roles), a.cfg.Staff.ProjectID)
}
