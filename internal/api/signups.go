package api

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/identity"
)

// Sign-ups (LAUNCH-01): the waitlist and invitations off it, for staff in the panel. Who may sign up
// at all is the platform setting directory.signup.

func (a *API) signupRoutes() []opRoute {
	return []opRoute{
		{"GET /v1/operator/waitlist", "customers.read", a.opWaitlist},
		{"POST /v1/operator/waitlist/{id}/invite", "customers.write", a.opWaitlistInvite},
	}
}

type waitlistRow struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Company   string    `json:"company"`
	Notes     string    `json:"notes"`
	Status    string    `json:"status"` // pending (address not confirmed), confirmed, invited
	CreatedAt time.Time `json:"created_at"`
}

func (a *API) opWaitlist(w http.ResponseWriter, r *http.Request, _ *caller) {
	if a.directory == nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Local accounts are not available on this installation.")
		return
	}
	rows, err := a.directory.SearchWaitlist(r.Context(), r.URL.Query().Get("q"))
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The waitlist couldn't be read.")
		return
	}
	out := make([]waitlistRow, 0, len(rows))
	for _, x := range rows {
		out = append(out, waitlistRow{x.ID, x.Email, x.Name, x.Company, x.Notes, x.Status, x.CreatedAt})
	}
	writeJSON(w, map[string]any{"signup": a.signupMode(r.Context()), "mail": a.directory.CanMail(), "items": out})
}

// opWaitlistInvite invites one confirmed row. The link is returned once, and mailed when mail is set up.
func (a *API) opWaitlistInvite(w http.ResponseWriter, r *http.Request, c *caller) {
	var in struct {
		Reason string `json:"reason"`
	}
	if !decodeStrict(w, r, &in) || !reasonOK(w, in.Reason) {
		return
	}
	if a.directory == nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Local accounts are not available on this installation.")
		return
	}
	id := r.PathValue("id")
	if !a.auditFirst(w, r, teamEntry(r, c, "signup.invite", "waitlist "+id+" · "+in.Reason)) {
		return
	}
	email, code, err := a.directory.InviteFromWaitlist(r.Context(), localUserID(c), id)
	switch {
	case errors.Is(err, identity.ErrNotFound):
		auth.Problem(w, http.StatusNotFound, "No such waitlist entry.")
		return
	case errors.Is(err, identity.ErrNotConfirmed):
		auth.Problem(w, http.StatusConflict, "This person hasn't confirmed their address yet, or was already invited.")
		return
	case err != nil:
		auth.Problem(w, http.StatusServiceUnavailable, "The invitation couldn't be created.")
		return
	}
	link := a.publicOrigin(r) + "/invite?" + url.Values{"code": {code}, "email": {email}}.Encode()
	mailed := a.directory.SendInvite(r.Context(), email, link) == nil
	writeJSON(w, map[string]any{"email": email, "link": link, "mailed": mailed})
}

// localUserID is the caller's local account id, or "" for a session from another issuer.
func localUserID(c *caller) string {
	if c == nil || c.id == nil {
		return ""
	}
	return strings.TrimPrefix(c.id.Sub, localSubPrefix)
}

// publicOrigin is the address people open the panel at: the configured origin, else this request's host.
func (a *API) publicOrigin(r *http.Request) string {
	if o := strings.TrimRight(a.cfg.Directory.Origin, "/"); o != "" {
		return o
	}
	return "https://" + r.Host
}
