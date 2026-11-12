package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/identity"
)

// Taking over the built-in setup account (REL-03). Whoever ran setup, a person or an AI assistant,
// signed in as the setup account; the person who will run the installation then sets its password and
// creates their own owner account. Only the setup account and owners see or do any of this.

// setupActor is the signed-in local user if it is the setup account or an active owner.
func (a *API) setupActor(ctx context.Context, r *http.Request) (uid string, isSetup bool, ok bool) {
	uid, signedIn := a.localUser(r)
	if !signedIn || a.db == nil {
		return "", false, false
	}
	var role string
	var builtin *string
	if err := a.db.Pool.QueryRow(ctx, `SELECT role, builtin FROM identity_users WHERE id::text=$1 AND active`, uid).Scan(&role, &builtin); err != nil || role != "owner" {
		return "", false, false
	}
	return uid, builtin != nil && *builtin == "setup", true
}

func (a *API) handoverState(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.localUser(r); !ok {
		auth.Problem(w, http.StatusUnauthorized, "Sign in to continue.")
		return
	}
	_, isSetup, ok := a.setupActor(r.Context(), r)
	if !ok || a.directory == nil {
		writeJSON(w, map[string]any{"pending": false})
		return
	}
	h, err := a.directory.SetupHandover(r.Context())
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The setup state couldn't be read.")
		return
	}
	writeJSON(w, map[string]any{"pending": h.Pending(), "you_are_setup": isSetup, "username": identity.SetupUsername,
		"password_set": h.PasswordSet, "owners": h.Owners})
}

// handoverPassword sets the setup account's password and signs out every other session of it.
func (a *API) handoverPassword(w http.ResponseWriter, r *http.Request) {
	uid, isSetup, ok := a.setupActor(r.Context(), r)
	if !a.mayHandOver(w, r, ok) {
		return
	}
	var in struct {
		Password string `json:"password"`
	}
	if !decodeStrict(w, r, &in) {
		return
	}
	setupID, err := a.directory.TakeOverSetup(r.Context(), uid, in.Password)
	switch {
	case errors.Is(err, identity.ErrPassword):
		auth.Problem(w, http.StatusBadRequest, "Use a password of at least 12 characters.")
		return
	case errors.Is(err, identity.ErrNotFound):
		auth.Problem(w, http.StatusNotFound, "This installation has no setup account.")
		return
	case errors.Is(err, identity.ErrNotOwner):
		auth.Problem(w, http.StatusForbidden, "Only the setup account or an owner can do this.")
		return
	case err != nil:
		log.Printf("setup takeover: %v", err)
		auth.Problem(w, http.StatusServiceUnavailable, "The password couldn't be saved. Try again.")
		return
	}
	keep := ""
	if isSetup {
		keep = a.flow.CurrentID(r)
	}
	ended, err := a.db.EndUserSessions(r.Context(), setupID, keep)
	if err != nil {
		log.Printf("setup takeover sessions: %v", err)
		auth.Problem(w, http.StatusServiceUnavailable, "The password is saved, but other sign-ins of the setup account couldn't be ended. Press save again.")
		return
	}
	a.auditSetup(r, uid, "setup.taken_over", fmt.Sprintf("setup account password set; %d other session(s) signed out, passkeys and recovery codes removed", ended))
	writeJSON(w, map[string]any{"password_set": true, "sessions_ended": ended})
}

// handoverOwner creates the person's own owner account.
func (a *API) handoverOwner(w http.ResponseWriter, r *http.Request) {
	uid, _, ok := a.setupActor(r.Context(), r)
	if !a.mayHandOver(w, r, ok) {
		return
	}
	var in struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if !decodeStrict(w, r, &in) {
		return
	}
	u, codes, err := a.directory.CreateOwner(r.Context(), uid, identity.OwnerInput{Username: in.Username, Email: in.Email, Name: in.Name, Password: in.Password})
	switch {
	case errors.Is(err, identity.ErrSetupInput):
		auth.Problem(w, http.StatusBadRequest, "Give a username (3 to 32 letters, digits, dots, dashes or underscores, not \"setup\"), a real email address and your name.")
		return
	case errors.Is(err, identity.ErrPassword):
		auth.Problem(w, http.StatusBadRequest, "Use a password of at least 12 characters.")
		return
	case errors.Is(err, identity.ErrConflict):
		auth.Problem(w, http.StatusConflict, "That username or email address is already in use.")
		return
	case errors.Is(err, identity.ErrNotOwner):
		auth.Problem(w, http.StatusForbidden, "Only the setup account or an owner can do this.")
		return
	case err != nil:
		log.Printf("setup owner: %v", err)
		auth.Problem(w, http.StatusServiceUnavailable, "The account couldn't be created. Try again.")
		return
	}
	a.auditSetup(r, uid, "setup.owner_created", "owner account "+u.Username+" created")
	writeJSON(w, map[string]any{"user_id": u.ID, "username": u.Username, "recovery_codes": codes})
}

// mayHandOver refuses a request from no session (401) or from anyone but the setup account or an owner (403).
func (a *API) mayHandOver(w http.ResponseWriter, r *http.Request, ok bool) bool {
	if _, signedIn := a.localUser(r); !signedIn {
		auth.Problem(w, http.StatusUnauthorized, "Sign in to continue.")
		return false
	}
	if !ok || a.directory == nil {
		auth.Problem(w, http.StatusForbidden, "Only the setup account or an owner can do this.")
		return false
	}
	return true
}

func (a *API) auditSetup(r *http.Request, uid, action, detail string) {
	if err := a.store().AddAudit(r.Context(), db.AuditEntry{Actor: LocalSubject(uid), Action: action, Detail: detail, IP: clientIP(r)}); err != nil {
		log.Printf("audit %s: %v", action, err)
	}
}
