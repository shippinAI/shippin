package hostclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// SiteReq is publish | unpublish | rollback of a workspace's public site (hostd site_handlers.go).
type SiteReq struct {
	Action  string `json:"action"`
	Slug    string `json:"slug"`
	Port    int    `json:"port"`
	Release string `json:"release,omitempty"`
	Path    string `json:"path,omitempty"`    // publish: app folder (host-side secret scan first)
	DryRun  bool   `json:"dry_run,omitempty"` // publish: scan only
}

type SiteRelease struct {
	ID string    `json:"id"`
	At time.Time `json:"at"`
}

type SiteState struct {
	Published bool          `json:"published"`
	URL       string        `json:"url,omitempty"`
	Current   string        `json:"current,omitempty"`
	Releases  []SiteRelease `json:"releases"`
	RRFlow    bool          `json:"rrflow_recorded"`
	Scan      string        `json:"scan,omitempty"`
	DryRun    bool          `json:"dry_run,omitempty"`
}

func (c *Client) Site(ctx context.Context, t, w, slug string) (*SiteState, error) {
	var out SiteState
	err := c.do(ctx, http.MethodGet, "/tenants/"+t+"/workspaces/"+w+"/site?slug="+url.QueryEscape(slug), nil, &out)
	return &out, err
}

func (c *Client) SiteAction(ctx context.Context, t, w string, r SiteReq) (*SiteState, error) {
	var out SiteState
	err := c.do(ctx, http.MethodPost, "/tenants/"+t+"/workspaces/"+w+"/site", r, &out)
	return &out, err
}
