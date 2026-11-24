package hostd

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func (s *Server) siteRoutes(m *http.ServeMux) {
	m.HandleFunc("GET /tenants/{t}/workspaces/{w}/site", s.withTenant(s.siteGet))
	m.HandleFunc("POST /tenants/{t}/workspaces/{w}/site", s.withTenant(s.sitePost))
}

// SiteReq is the console's publish/unpublish/rollback request.
type SiteReq struct {
	Action  string `json:"action"` // publish | unpublish | rollback
	Slug    string `json:"slug"`
	Port    int    `json:"port"`
	Release string `json:"release"` // rollback target; empty = the one before the current
	Path    string `json:"path"`    // publish: the app folder, secret-scanned on the host first
	DryRun  bool   `json:"dry_run"` // publish: validate + scan only; no snapshot, no route
}

var siteNow = time.Now

func (s *Server) siteState(ctx context.Context, p place, w, host string) SiteState {
	st := SiteState{Releases: []Release{}}
	if rs, err := p.siteReleases(ctx, w); err == nil {
		st.Releases = rs
	}
	if cur, err := p.do(ctx, "config", "get", SiteInstance(w), "user.shippin.site_release"); err == nil {
		st.Current = strings.TrimSpace(cur)
	}
	if _, err := s.caddy(ctx, "GET", "/id/"+siteRouteID(w), nil); err == nil && host != "" {
		st.Published, st.URL = true, "https://"+host
	}
	return st
}

func (s *Server) siteGet(w http.ResponseWriter, r *http.Request) {
	p, err := s.placeOf(r.Context(), r.PathValue("t"), r.PathValue("w"))
	if err != nil {
		fail(w, http.StatusConflict, "workspace is not reachable")
		return
	}
	host, _ := s.SiteHost(r.URL.Query().Get("slug"))
	reply(w, s.siteState(r.Context(), p, r.PathValue("w"), host))
}

func (s *Server) sitePost(w http.ResponseWriter, r *http.Request) {
	var in SiteReq
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	host, err := s.SiteHost(in.Slug)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	ws := r.PathValue("w")
	p, err := s.placeOf(r.Context(), r.PathValue("t"), ws)
	if err != nil {
		fail(w, http.StatusConflict, "workspace is not reachable")
		return
	}
	ctx := r.Context()
	rel := ""
	switch in.Action {
	case "publish":
		// B2: one-click Publish uses the GitHub-publish host scan path (pinned gitleaks on the host).
		if err = s.validPreview(in.Port); err == nil {
			err = s.hostScanFolder(ctx, p, Instance(ws), in.Path)
		}
		if err == nil && in.DryRun {
			st := s.siteState(ctx, p, ws, host)
			st.Scan, st.DryRun = "clean", true
			reply(w, st)
			return
		}
		if err == nil {
			rel, err = s.Publish(ctx, p, ws, host, in.Port, siteNow())
		}
	case "rollback":
		cur := s.siteState(ctx, p, ws, host).Current
		rel, err = s.Rollback(ctx, p, ws, host, in.Release, cur, in.Port)
	case "unpublish":
		err = s.Unpublish(ctx, p, ws)
	default:
		fail(w, http.StatusBadRequest, "action must be publish, unpublish or rollback")
		return
	}
	if err != nil {
		fail(w, http.StatusConflict, err.Error())
		return
	}
	st := s.siteState(ctx, p, ws, host)
	if in.Action == "publish" {
		st.Scan = "clean"
	}
	st.RRFlow = recordSite(ctx, p, ws, in.Action, host, rel)
	reply(w, st)
}

// recordSite appends the action to the project's rrflow/state decision log from inside the
// devspace (the rrflow client there is bound to the project's instance); best effort.
func recordSite(ctx context.Context, p place, w, action, host, rel string) bool {
	_, err := p.do(ctx, "exec", Instance(w), "--", "sh", "-c",
		`command -v rrflow >/dev/null && [ -n "$RRFLOW_URL" ] && exec rrflow decision add --action "$1" --effect allow --resource "$2" --reason "$3"`,
		"site", "site."+action, host, rel)
	return err == nil
}
