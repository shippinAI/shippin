package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/servers"
)

var serverSlug = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

// serverStore is the CP-P1 registry; the clock comes from config, never from code.
func (a *API) serverStore() *servers.Store {
	c := a.cfg.Servers
	return &servers.Store{Pool: a.db.Pool, Clock: servers.Clock{Interval: time.Duration(c.HeartbeatSeconds) * time.Second, Missed: c.MissedBeats}}
}

// serverRoutes (CP-P1): org owners/admins manage servers; hostd enrolls with a single-use token
// and then beats with its own credential (no user session on those two routes).
func (a *API) serverRoutes(mux Router) {
	mux.HandleFunc("GET /v1/tenants/{tenantId}/servers", a.flow.Require(a.serverAdmin(a.listServers)))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/servers", a.flow.Require(a.serverAdmin(a.createServer)))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/servers/{serverId}/enroll-token", a.flow.Require(a.serverAdmin(a.issueServerToken)))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/servers/{serverId}/revoke", a.flow.Require(a.serverAdmin(a.revokeServer)))
	mux.HandleFunc("POST /v1/servers/enroll", a.enrollServer)
	mux.HandleFunc("POST /v1/servers/heartbeat", a.serverHeartbeat)
}

type serverHandler func(http.ResponseWriter, *http.Request, *caller, *tenantView)

// serverAdmin gates every admin route: the feature is on and the caller is an org owner/admin.
func (a *API) serverAdmin(h serverHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !a.cfg.Servers.Enabled {
			auth.Problem(w, http.StatusNotFound, "Servers aren't enabled.")
			return
		}
		c, ok := a.resolve(w, r)
		if !ok {
			return
		}
		t, ok := c.tenant(w, r.PathValue("tenantId"))
		if !ok {
			return
		}
		if !c.canDecide(t) {
			auth.Problem(w, http.StatusForbidden, "Managing servers needs the org owner or admin role.")
			return
		}
		h(w, r, c, t)
	}
}

func (a *API) listServers(w http.ResponseWriter, r *http.Request, _ *caller, t *tenantView) {
	items, err := a.serverStore().List(r.Context(), t.ID)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Servers are unavailable right now.")
		return
	}
	writeJSON(w, map[string]any{"items": items, "heartbeat_seconds": a.cfg.Servers.HeartbeatSeconds,
		"missed_beats": a.cfg.Servers.MissedBeats})
}

type serverCreate struct {
	Slug  string `json:"slug"`
	Label string `json:"label"`
	Site  string `json:"site"`
}

func (a *API) createServer(w http.ResponseWriter, r *http.Request, c *caller, t *tenantView) {
	a.idempotent(w, r, t.ID+":"+c.id.Sub, func(w http.ResponseWriter, r *http.Request) { a.doCreateServer(w, r, c, t) })
}

func (a *API) doCreateServer(w http.ResponseWriter, r *http.Request, c *caller, t *tenantView) {
	var in serverCreate
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil {
		auth.Problem(w, http.StatusBadRequest, "Send a JSON body with slug, label and site.")
		return
	}
	in.Label = strings.TrimSpace(in.Label)
	if n := utf8.RuneCountInString(in.Label); !serverSlug.MatchString(in.Slug) || !serverSlug.MatchString(in.Site) ||
		n < 1 || n > 120 || strings.ContainsAny(in.Label, "\r\n\t") {
		auth.Problem(w, http.StatusBadRequest, "Slug and site: lowercase letters, digits and dashes (max 40). Label: 1 to 120 characters.")
		return
	}
	s := a.serverStore()
	if n, err := s.Count(r.Context(), t.ID); err != nil || n >= a.cfg.Servers.MaxPerTenant {
		auth.Problem(w, http.StatusConflict, "This org has reached its server limit.")
		return
	}
	e := db.AuditEntry{TenantID: t.ID, Actor: c.id.Sub, ActorName: displayName(c), Action: "server.create",
		Detail: in.Slug + " " + in.Site, IP: clientIP(r)}
	if !a.auditFirst(w, r, e) {
		return
	}
	v, err := s.Create(r.Context(), t.ID, in.Slug, in.Label, in.Site, c.id.Sub)
	if errors.Is(err, servers.ErrSlugTaken) {
		auth.Problem(w, http.StatusConflict, "A server with that slug already exists.")
		return
	} else if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The server couldn't be saved. Try again.")
		return
	}
	writeStatus(w, http.StatusCreated, v)
}
