package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"

	"github.com/jackc/pgx/v5/pgconn"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

var teamNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,38}[a-z0-9]$`)

func (a *API) createTeam(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.teamCtx(w, r, true)
	if !ok {
		return
	}
	var in struct{ Name, Description string }
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048)).Decode(&in) != nil || !teamNameRe.MatchString(in.Name) || len(in.Description) > 300 {
		auth.Problem(w, http.StatusBadRequest, "Team names are 2-40 lowercase letters, digits and dashes; descriptions up to 300 characters.")
		return
	}
	id, err := a.db.CreateTeam(r.Context(), t.ID, in.Name, in.Description, c.id.Sub, a.teamAudit(r, c, t, "tenant.team_create", in.Name))
	var pe *pgconn.PgError
	switch {
	case errors.As(err, &pe) && pe.Code == "23505":
		auth.Problem(w, http.StatusConflict, "A team with that name already exists.")
	case err != nil:
		auth.Problem(w, http.StatusServiceUnavailable, "The team couldn't be created right now.")
	default:
		writeStatus(w, http.StatusCreated, map[string]string{"id": id, "name": in.Name})
	}
}

func (a *API) deleteTeam(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.teamCtx(w, r, true)
	if !ok {
		return
	}
	id := r.PathValue("teamId")
	a.teamWrite(w, a.db.DeleteTeam(r.Context(), t.ID, id, a.teamAudit(r, c, t, "tenant.team_delete", id)))
}

// setTeamMember adds an org member to a team; people outside the org can't be added.
func (a *API) setTeamMember(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.teamCtx(w, r, true)
	if !ok {
		return
	}
	var in struct{ Role string }
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 512)).Decode(&in)
	if in.Role == "" {
		in.Role = "member"
	}
	if in.Role != "member" && in.Role != "maintainer" {
		auth.Problem(w, http.StatusBadRequest, "Team role is member or maintainer.")
		return
	}
	if _, _, ok := a.memberState(w, r, t); !ok {
		return
	}
	uid, team := r.PathValue("userId"), r.PathValue("teamId")
	e := a.teamAudit(r, c, t, "tenant.team_member_set", team+" "+uid+" "+in.Role)
	a.teamWrite(w, a.db.SetTeamMember(r.Context(), t.ID, team, uid, in.Role, c.id.Sub, e))
}

func (a *API) removeTeamMember(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.teamCtx(w, r, true)
	if !ok {
		return
	}
	uid, team := r.PathValue("userId"), r.PathValue("teamId")
	a.teamWrite(w, a.db.RemoveTeamMember(r.Context(), t.ID, team, uid, a.teamAudit(r, c, t, "tenant.team_member_remove", team+" "+uid)))
}

func (a *API) teamWrite(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, db.ErrNotFound):
		auth.Problem(w, http.StatusNotFound, "That team or member isn't in this tenant.")
	case err != nil:
		auth.Problem(w, http.StatusServiceUnavailable, "The team couldn't be changed right now.")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
