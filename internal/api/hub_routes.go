package api

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"shippin.cloud/console/internal/approval"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/hub"
)

// RegisterHub mounts the agent checkout board (/v1/hub). Who may approve is approval policy
// (config/shippin.example.json, plus HUB_OWNER_SUB / HUB_OWNER_EMAIL copied onto that policy). The board
// stays unmounted when there is no database, or neither a hub.decide rule nor those env values.
func (a *API) RegisterHub(mux Router) {
	if a.db == nil {
		log.Printf("hub: no database; /v1/hub is not mounted")
		return
	}
	pol := a.cfg.Approval
	owner, email := os.Getenv("HUB_OWNER_SUB"), os.Getenv("HUB_OWNER_EMAIL")
	if !pol.Enabled || !pol.HasAction("hub.decide") {
		if owner == "" && email == "" {
			log.Printf("hub: no hub.decide rule and no HUB_OWNER_*; /v1/hub is not mounted")
			return
		}
	}
	lease := 10 * time.Minute
	if d, err := time.ParseDuration(os.Getenv("HUB_LEASE")); err == nil && d > 0 {
		lease = d
	}
	live := hub.NewLive()
	go live.Run(context.Background(), a.db.Pool)
	h := &hub.Handler{Store: &hub.Store{Pool: a.db.Pool}, OwnerSub: owner, OwnerEmail: email,
		Policy: &pol, Lease: lease, Who: a.hubWho, Live: live}
	h.Register(requireAll{mux, a.flow})
}

func (a *API) hubWho(w http.ResponseWriter, r *http.Request) (*hub.Principal, bool) {
	c, ok := a.caller(w, r)
	if !ok {
		return nil, false
	}
	return &hub.Principal{Sub: c.id.Sub, Name: c.id.Name, OrgID: c.id.OrgID,
		Email: c.id.Email, EmailVerified: c.id.EmailVerified, Roles: approverRoles(a, c)}, true
}

func approverRoles(a *API, c *caller) []string {
	var roles []string
	if (c.founder || c.installOwner) && a.cfg.Identity.FounderRole != "" {
		roles = append(roles, a.cfg.Identity.FounderRole)
	}
	if c.op != "" {
		roles = append(roles, c.op)
	}
	return roles
}

func approverSubject(a *API, c *caller) approval.Subject {
	return approval.Subject{Sub: c.id.Sub, Roles: approverRoles(a, c), Email: c.id.Email, EmailVerified: c.id.EmailVerified}
}

// requireAll wraps every route in the session/bearer requirement.
type requireAll struct {
	mux  Router
	flow *auth.Flow
}

func (m requireAll) HandleFunc(p string, h func(http.ResponseWriter, *http.Request)) {
	m.mux.HandleFunc(p, m.flow.Require(h))
}
