package api

import (
	"context"
	"log"
	"time"

	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/mail"
)

// SetNotifier enables e-mail notices (nil keeps them off).
func (a *API) SetNotifier(n *mail.Notifier) { a.notify = n }

// notifyLater resolves recipients and queues a notice off the request path; the action has
// already succeeded and been audited, so a lookup or queue failure is only logged.
func (a *API) notifyLater(what string, fn func(ctx context.Context) error) {
	if a.notify == nil {
		return
	}
	run := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := fn(ctx); err != nil {
			log.Printf("notice %s: %v", what, err)
		}
	}
	if a.notifySync {
		run()
		return
	}
	go run()
}

func (a *API) consoleURL(path string) string { return "https://" + a.cfg.ConsoleHost + path }

func grantFacts(g db.Grant) [][2]string {
	scope := "Whole project space"
	if g.WorkspaceName != "" {
		scope = "Workspace " + g.WorkspaceName
	}
	f := [][2]string{{"Project space", g.TenantName}, {"Scope", scope}, {"Requested by", g.RequesterName}, {"Reason", g.Reason}}
	if g.Category != "" {
		f = append(f, [2]string{"Category", g.Category})
	}
	if g.ExpiresAt != nil {
		f = append(f, [2]string{"Ends", g.ExpiresAt.UTC().Format("Jan 2, 2006 15:04 UTC")})
	} else {
		f = append(f, [2]string{"Duration", (time.Duration(g.DurationS) * time.Second).String()})
	}
	return f
}

func (a *API) tenantOrg(ctx context.Context, id string) (string, error) {
	t, err := a.db.TenantByID(ctx, id)
	if err != nil || t == nil {
		return "", err
	}
	return t.IdpOrgID, nil
}

// noticeToManagers queues kind for the tenant's owners/admins, delivered no earlier than after.
func (a *API) noticeToManagers(ctx context.Context, kind string, g db.Grant, after time.Time, vars map[string]string) error {
	org, err := a.tenantOrg(ctx, g.TenantID)
	if err != nil || org == "" {
		return err
	}
	to, err := a.recipientsOf(ctx, org)
	if err != nil {
		return err
	}
	_, err = a.notify.Queue(ctx, mail.Notice{Kind: kind, Vars: vars, Facts: grantFacts(g), Alert: g.Kind == "breakglass",
		URL: a.consoleURL("/t/" + g.TenantID + "/settings")}, to, g.ID, after)
	return err
}

// noticeToRequester queues kind for the admin who asked for the grant.
func (a *API) noticeToRequester(ctx context.Context, kind string, g db.Grant, vars map[string]string) error {
	to, err := a.emailOf(ctx, g.Requester)
	if err != nil || to == "" {
		return err
	}
	_, err = a.notify.Queue(ctx, mail.Notice{Kind: kind, Vars: vars, Facts: grantFacts(g), URL: a.consoleURL("/admin/access")},
		[]string{to}, g.ID, time.Time{})
	return err
}

func (a *API) recipientsOf(ctx context.Context, org string) ([]string, error) {
	if a.lookupManagers != nil {
		return a.lookupManagers(ctx, org)
	}
	return a.managerEmails(ctx, org)
}

func (a *API) emailOf(ctx context.Context, sub string) (string, error) {
	if a.lookupUser != nil {
		return a.lookupUser(ctx, sub)
	}
	return a.userEmail(ctx, sub)
}
