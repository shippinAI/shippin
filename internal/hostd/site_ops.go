package hostd

import (
	"context"
	"fmt"
	"time"
)

type snapList = []struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// siteReleases lists a workspace's pub-* snapshots, newest first.
func (p place) siteReleases(ctx context.Context, w string) ([]Release, error) {
	var sn snapList
	if err := p.query(ctx, "/1.0/instances/"+Instance(w)+"/snapshots?recursion=1", &sn); err != nil {
		return nil, err
	}
	return releases(sn), nil
}

// deploySite (re)creates s-<w> from snapshot rel, drops forwarded host ports it inherited (the
// devspace keeps them), starts it and returns its checked address.
func (p place) deploySite(ctx context.Context, w, rel string) (string, error) {
	si := SiteInstance(w)
	if p.has(ctx, "info", si) {
		if _, err := p.do(ctx, "delete", "--force", si); err != nil {
			return "", err
		}
	}
	if _, err := p.do(ctx, "copy", Instance(w)+"/"+rel, si, "--stateless"); err != nil {
		return "", err
	}
	var dev struct {
		Devices map[string]map[string]string `json:"devices"`
	}
	if err := p.query(ctx, "/1.0/instances/"+si, &dev); err != nil {
		return "", err
	}
	for name, d := range dev.Devices {
		if d["type"] == "proxy" {
			if _, err := p.do(ctx, "config", "device", "remove", si, name); err != nil {
				return "", err
			}
		}
	}
	if _, err := p.do(ctx, "config", "set", si, "user.shippin.site_release="+rel); err != nil {
		return "", err
	}
	if _, err := p.do(ctx, "start", si); err != nil && !isRunning(err) {
		return "", err
	}
	return p.waitIP(ctx, si)
}

// prune keeps the newest maxReleases pub-* snapshots, never the one being served.
func (p place) prune(ctx context.Context, w, keep string) {
	rs, err := p.siteReleases(ctx, w)
	if err != nil {
		return
	}
	for i, r := range rs {
		if i >= maxReleases && r.ID != keep {
			_, _ = p.do(ctx, "delete", Instance(w)+"/"+r.ID)
		}
	}
}

// Publish snapshots the workspace as a new release and serves it at host:port.
func (s *Server) Publish(ctx context.Context, p place, w, host string, port int, now time.Time) (string, error) {
	if err := s.validPreview(port); err != nil {
		return "", err
	}
	rel := releaseName(now)
	if _, err := p.do(ctx, "snapshot", "create", Instance(w), rel); err != nil {
		return "", err
	}
	if err := s.serve(ctx, p, w, host, rel, port); err != nil {
		return "", err
	}
	p.prune(ctx, w, rel)
	return rel, nil
}

// Rollback serves an existing release (default: the one before the current).
func (s *Server) Rollback(ctx context.Context, p place, w, host, rel, current string, port int) (string, error) {
	rs, err := p.siteReleases(ctx, w)
	if err != nil {
		return "", err
	}
	if rel == "" {
		for i, r := range rs {
			if r.ID == current && i+1 < len(rs) {
				rel = rs[i+1].ID
			}
		}
	}
	found := false
	for _, r := range rs {
		found = found || (r.ID == rel && rel != "")
	}
	if !found {
		return "", fmt.Errorf("no earlier release to roll back to")
	}
	return rel, s.serve(ctx, p, w, host, rel, port)
}

func (s *Server) serve(ctx context.Context, p place, w, host, rel string, port int) error {
	ip, err := p.deploySite(ctx, w, rel)
	if err != nil {
		return err
	}
	_, _ = s.caddy(ctx, "DELETE", "/id/"+siteRouteID(w), nil)
	_, err = s.caddy(ctx, "PUT", fmt.Sprintf("/config/apps/http/servers/%s/routes/0", s.cfg.Edge.Server),
		s.siteRoute(w, host, ip, port))
	return err
}

// Unpublish removes the public route and stops the site instance; releases are kept for re-publish.
func (s *Server) Unpublish(ctx context.Context, p place, w string) error {
	_, _ = s.caddy(ctx, "DELETE", "/id/"+siteRouteID(w), nil)
	if p.has(ctx, "info", SiteInstance(w)) {
		_, err := p.do(ctx, "stop", "--force", SiteInstance(w))
		return err
	}
	return nil
}
