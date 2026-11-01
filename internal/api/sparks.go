package api

import (
	"net/http"
	"net/url"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/config"
)

// sparks never exposes the fleet to another tenant or to a caller without the
// same permission used by management-host forward_auth. No upstream credentials
// or private network addresses are part of this response.
func (a *API) sparks(w http.ResponseWriter, r *http.Request) {
	c, ok := a.resolve(w, r)
	if !ok {
		return
	}
	t, ok := c.tenant(w, r.PathValue("tenantId"))
	if !ok {
		return
	}
	if !t.member() || !a.can(r, c, "host.admin_tools") {
		auth.Problem(w, http.StatusForbidden, "Spark access requires tenant membership and permission to open platform tools.")
		return
	}
	if t.ID != a.cfg.Sparks.TenantID {
		writeJSON(w, map[string]any{"nodes": []config.SparkNode{}})
		return
	}
	writeJSON(w, a.cfg.Sparks)
}

// sparkHost identifies only explicitly published destinations. It is checked
// before the broader platform admin-host rule, so knowing a Spark URL cannot
// bypass the owning tenant's membership requirement.
func (a *API) sparkHost(host string) bool {
	match := func(e config.SparkEndpoint) bool {
		u, err := url.Parse(e.URL)
		return e.State == "published" && err == nil && u.Host == host
	}
	if match(a.cfg.Sparks.ModelAPI) {
		return true
	}
	for _, n := range a.cfg.Sparks.Nodes {
		for _, e := range n.Management {
			if match(e) {
				return true
			}
		}
	}
	return false
}

func (a *API) sparkAllowed(r *http.Request, tok, host string) bool {
	c, err := a.callerFor(r, tok)
	return err == nil && a.sparkPermit(r, c, host)
}

func (a *API) sparkPermit(r *http.Request, c *caller, host string) bool {
	if !a.sparkHost(host) {
		return false
	}
	for i := range c.tenants {
		t := &c.tenants[i]
		if t.ID == a.cfg.Sparks.TenantID && t.member() {
			return a.adminPermit(r, c, host)
		}
	}
	return false
}
