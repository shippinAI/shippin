package hostd

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var (
	hostRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,48}$`)
	fqdnRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)
)

// ValidDomain checks a custom domain attached to a workspace.
func ValidDomain(h string) error {
	if len(h) > 253 || !fqdnRe.MatchString(h) {
		return fmt.Errorf("invalid domain")
	}
	return nil
}

// ValidHost checks a workspace hostname is <label>.<ws base domain>.
func (s *Server) ValidHost(h string) error {
	sub, ok := strings.CutSuffix(h, "."+s.cfg.Edge.BaseDomain)
	if !ok || !hostRe.MatchString(sub) {
		return fmt.Errorf("invalid workspace host")
	}
	return nil
}

func up(dial string) map[string]any {
	return map[string]any{"handler": "reverse_proxy", "upstreams": []any{map[string]string{"dial": dial}}}
}

// route builds the workspace IDE route: /_shippin/* → console; with an agent terminal (M4),
// <term_base>/* → forward_auth then ttyd; everything else → forward_auth then IDE.
func (s *Server) route(w string, hosts []string, ip string) map[string]any {
	var paths []any
	if a := s.cfg.Agent; a.enabled() && a.TermPort > 0 {
		paths = append(paths, map[string]any{"match": []any{map[string]any{"path": []string{a.TermBase + "/*"}}},
			"handle": s.handle(fmt.Sprintf("%s:%d", ip, a.TermPort), true, false)})
	}
	return s.routeWith("ws-"+w, hosts, s.handle(fmt.Sprintf("%s:%d", ip, s.cfg.Edge.IDEPort), true, false), paths...)
}

// proxyRoute is one Caddy route. private adds forward_auth (owner sign-in via the console);
// devServer rewrites Host to the upstream so dev servers (Vite, Next…) accept the request.
func (s *Server) proxyRoute(id string, hosts []string, upstream string, private, devServer bool) map[string]any {
	return s.routeWith(id, hosts, s.handle(upstream, private, devServer))
}

// handle is the handler chain for one upstream: optional forward_auth, then the proxy with the
// console's workspace cookie stripped.
func (s *Server) handle(upstream string, private, devServer bool) []any {
	e := s.cfg.Edge
	target := up(upstream)
	// The console's workspace session cookie never reaches workspace code (IDE or dev server).
	req := map[string]any{"replace": map[string]any{"Cookie": []map[string]string{
		{"search_regexp": `(^|;\s*)` + regexp.QuoteMeta(e.WSCookie) + `=[^;]*`, "replace": ""}}}}
	if devServer {
		req["set"] = map[string][]string{"Host": {"{http.reverse_proxy.upstream.hostport}"}, "X-Forwarded-Host": {"{http.request.host}"}}
	}
	target["headers"] = map[string]any{"request": req}
	if !private {
		return []any{target}
	}
	auth := up(e.Console)
	auth["rewrite"] = map[string]string{"method": "GET", "uri": "/auth/ws-check"}
	auth["headers"] = map[string]any{"request": map[string]any{"set": map[string][]string{
		"X-Forwarded-Method": {"{http.request.method}"}, "X-Forwarded-Uri": {"{http.request.uri}"}}}}
	auth["handle_response"] = []any{map[string]any{"match": map[string]any{"status_code": []int{2}},
		"routes": []any{map[string]any{"handle": []any{map[string]string{"handler": "vars"}}}}}}
	return []any{auth, target}
}

// routeWith: /_shippin/* → console, then the extra path routes, then the default handler.
func (s *Server) routeWith(id string, hosts []string, def []any, paths ...any) map[string]any {
	routes := []any{map[string]any{"match": []any{map[string]any{"path": []string{"/_shippin/*"}}}, "handle": []any{up(s.cfg.Edge.Console)}}}
	routes = append(append(routes, paths...), map[string]any{"handle": def})
	return map[string]any{
		"@id":      id,
		"match":    []any{map[string]any{"host": hosts}},
		"handle":   []any{map[string]any{"handler": "subroute", "routes": routes}},
		"terminal": true,
	}
}

// caddy calls the edge's local admin API (localhost only) with a fixed method and path.
func (s *Server) caddy(ctx context.Context, method, path string, body any) (string, error) {
	raw := ""
	if body != nil {
		b, _ := json.Marshal(body)
		raw = string(b)
	}
	return runner(ctx, raw, "incus", "exec", s.cfg.Edge.Instance, "--project="+s.cfg.Platform, "--",
		"curl", "-sf", "-X", method, "-H", "Content-Type: application/json", "--data-binary", "@-",
		s.cfg.Edge.CaddyAdmin+path)
}

// PublishRoute replaces any existing route for the workspace and inserts it first.
// Extra hosts are custom domains; each is validated.
func (s *Server) PublishRoute(ctx context.Context, w, host, ip string, extra []string) error {
	if err := s.ValidHost(host); err != nil {
		return err
	}
	hosts := []string{host}
	for _, d := range extra {
		if ValidDomain(d) != nil || s.cfg.reservedDomain(d) {
			return fmt.Errorf("domain %q is not allowed", d)
		}
		hosts = append(hosts, d)
	}
	_ = s.UnpublishRoute(ctx, w)
	_, err := s.caddy(ctx, "PUT", fmt.Sprintf("/config/apps/http/servers/%s/routes/0", s.cfg.Edge.Server), s.route(w, hosts, ip))
	return err
}

func (s *Server) UnpublishRoute(ctx context.Context, w string) error {
	_, err := s.caddy(ctx, "DELETE", "/id/ws-"+w, nil)
	return err
}
