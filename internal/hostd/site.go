package hostd

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// A site is the one-click public deployment of a workspace app: Publish snapshots the workspace
// (pub-<unix>), copies that snapshot into a frozen site instance s-<w>, and points a public edge
// route (no forward_auth) at it. Editing the devspace never changes the published site; Rollback
// re-copies an older pub-* snapshot; Unpublish drops the route and stops the site instance.
// Releases are the pub-* snapshots themselves, so the history survives hostd restarts.

const (
	sitePrefix  = "pub-"
	maxReleases = 10
)

var slugRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$`)

// SiteInstance is the frozen copy of a workspace that serves its published site.
func SiteInstance(w string) string { return "s-" + w }

// siteRouteID is the edge route @id of a workspace's public site.
func siteRouteID(w string) string { return "site-" + w }

// SiteHost is <slug>--site.<ws base>: one DNS label under the *.ws wildcard certificate,
// distinct from IDE hosts and <port>--<label> preview hosts.
func (s *Server) SiteHost(slug string) (string, error) {
	if !slugRe.MatchString(slug) {
		return "", fmt.Errorf("site name must be 1-32 lowercase letters, digits or dashes")
	}
	return slug + "--site." + s.cfg.Edge.BaseDomain, nil
}

// Release is one published version of a site.
type Release struct {
	ID string    `json:"id"`
	At time.Time `json:"at"`
}

// releaseName is the snapshot name for a release published at t.
func releaseName(t time.Time) string { return sitePrefix + strconv.FormatInt(t.Unix(), 10) }

// releases parses `incus query .../snapshots?recursion=1` output into pub-* releases, newest first.
func releases(snaps []struct {
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}) []Release {
	out := []Release{}
	for _, sn := range snaps {
		if len(sn.Name) > len(sitePrefix) && sn.Name[:len(sitePrefix)] == sitePrefix {
			out = append(out, Release{ID: sn.Name, At: sn.CreatedAt})
		}
	}
	for i := 1; i < len(out); i++ { // insertion sort, newest first (few items)
		for j := i; j > 0 && out[j].At.After(out[j-1].At); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// siteRoute is a public route: the console workspace cookie is still stripped, no auth gate.
func (s *Server) siteRoute(w, host, ip string, port int) map[string]any {
	return s.proxyRoute(siteRouteID(w), []string{host}, fmt.Sprintf("%s:%d", ip, port), false, true)
}

// SiteState is what the panel shows for a workspace's site.
type SiteState struct {
	Published bool      `json:"published"`
	URL       string    `json:"url,omitempty"`
	Current   string    `json:"current,omitempty"`
	Releases  []Release `json:"releases"`
	RRFlow    bool      `json:"rrflow_recorded"`
	Scan      string    `json:"scan,omitempty"`    // "clean" after the host-side secret scan passed
	DryRun    bool      `json:"dry_run,omitempty"` // publish dry run: scanned only, nothing published
}
