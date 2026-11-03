package api

import (
	"context"
	"log"
	"slices"
	"time"

	"shippin.cloud/console/internal/db"
)

// accountWindow throttles the account upsert to once per user per window (callerFor runs on
// every request, including forward_auth checks).
const accountWindow = 10 * time.Minute

// syncAccount upserts the caller's accounts row and links it as owner of every tenant whose
// IdP org it owns (members only; staff grants never make an owner). This also backfills
// tenants created before accounts existed, on the owner's next request.
func (a *API) syncAccount(ctx context.Context, c *caller) {
	if a.db == nil || c == nil || c.id == nil || c.id.Sub == "" {
		return
	}
	if !a.seen.due(c.id.Sub+"|account", accountWindow, time.Now()) {
		return
	}
	acc, err := a.db.UpsertAccount(ctx, c.id.Sub, c.id.Email)
	if err != nil {
		log.Printf("account upsert for %s: %v", c.id.Sub, err)
		a.seen.forget(c.id.Sub + "|account")
		return
	}
	for _, t := range c.tenants {
		if t.grants == nil && slices.Contains(t.Roles, a.cfg.Roles.Owner) {
			if err := a.db.AddTenantOwner(ctx, t.ID, acc); err != nil {
				log.Printf("tenant owner %s/%s: %v", t.ID, acc, err)
			}
		}
	}
}

// recordOwner links the onboarding user as owner of the tenant they just created. It runs even
// when provisioning failed: the tenant row exists and the retry keeps the same owner.
func (a *API) recordOwner(ctx context.Context, c *caller, t *db.Tenant) {
	if t == nil || t.ID == "" {
		return
	}
	acc, err := a.db.UpsertAccount(ctx, c.id.Sub, c.id.Email)
	if err == nil {
		err = a.db.AddTenantOwner(ctx, t.ID, acc)
	}
	if err != nil {
		log.Printf("onboarding: owner of %s not recorded: %v", t.ID, err)
	}
}
