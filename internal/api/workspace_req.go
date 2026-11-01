package api

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hostclient"
)

var slugRe = regexp.MustCompile(`[^a-z0-9-]+`)

// wsHost builds <name>-<short>.<base domain>; short = last 6 hex of the workspace UUID.
func (a *API) wsHost(name, id string) string {
	slug := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(slug) > 30 {
		slug = slug[:30]
	}
	return fmt.Sprintf("%s-%s.%s", slug, id[len(id)-6:], a.cfg.Workspaces.BaseDomain)
}

// hostReq is the full desired state of a workspace sent to shippin-hostd.
func (a *API) hostReq(t *tenantView, w db.Workspace) hostclient.WorkspaceReq {
	rules := w.Firewall
	if rules == nil {
		rules = []db.Rule{}
	}
	prev := w.Previews
	if prev == nil {
		prev = []db.Preview{}
	}
	req := hostclient.WorkspaceReq{Name: w.Name, Plan: t.Plan, Keys: w.SSHKeys, Port: w.SSHPort, SSHIP: t.SSHIP,
		Host: w.Host, Domains: w.Domains, Rules: rules, Size: w.Size, Repo: w.Repo, Previews: prev,
		Project: w.ProjectID, Environment: w.EnvironmentID, Boundary: w.BoundaryID}
	if w.Template != "" {
		if tpl, err := a.db.Template(context.Background(), w.Template); err == nil && tpl != nil {
			req.Image, req.Startup = tpl.Image, tpl.Startup
		}
	}
	// Provider credentials belong to the owner's isolated runner, not a workspace
	// that tenant managers or support grants can also open.
	return req
}

func (a *API) findWorkspace(r interface{ PathValue(string) string }, ws []workspaceView) *workspaceView {
	for i := range ws {
		if ws[i].ID == r.PathValue("workspaceId") {
			return &ws[i]
		}
	}
	return nil
}
