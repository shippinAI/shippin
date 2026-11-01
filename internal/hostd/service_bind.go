package hostd

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// BindReq binds a service instance to one workspace of the same project.
type BindReq struct {
	Project   string `json:"project_id"`
	Workspace string `json:"workspace_id"`
	Shared    bool   `json:"shared"` // D36 VM-wide instance: the binding uses namespace Project
}

const rrflowProfile = `[ -r "$HOME/.shippin/rrflow.env" ] && . "$HOME/.shippin/rrflow.env"` + "\n"

// BindRRFlow mints a fresh binding token in hostd's memory, gives its hash to the instance and
// the token itself to the workspace user (~/.shippin/rrflow.token, 0600). The token is never
// returned, logged or stored by the control plane.
func (s *Server) BindRRFlow(ctx context.Context, t, si, sb string, r BindReq) error {
	if ValidUUID(sb) != nil || ValidUUID(r.Workspace) != nil {
		return fmt.Errorf("invalid binding or workspace id")
	}
	at, err := s.servicePlace(ctx, t, r.Project, nil, r.Shared)
	if err != nil {
		return err
	}
	ns := []string{}
	if r.Shared {
		ns = append(ns, r.Project)
	}
	wat, err := s.placeOf(ctx, t, r.Workspace)
	if err != nil || !wat.vm || wat.pj != r.Project {
		return fmt.Errorf("workspace is not in this project on the boundary VM")
	}
	c, inst, ws := s.cfg.Services.RRFlow, ServiceInstance(si), Instance(r.Workspace)
	ip, err := at.waitIP(ctx, inst)
	if err != nil || at.checkIP(ip) != nil {
		return fmt.Errorf("service instance is not running")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	tok := base64.RawURLEncoding.EncodeToString(raw)
	if _, err := at.inc(ctx, tok+"\n", append([]string{"exec", inst, "--env", "RRFLOW_DATA=" + c.DataDir, "--",
		"runuser", "-u", "rrflow", "--", "/usr/local/bin/rrflow-state", "bind", sb, r.Workspace}, ns...)...); err != nil {
		return fmt.Errorf("binding not recorded in the service")
	}
	bin, err := os.ReadFile(c.Binary)
	if err != nil {
		return fmt.Errorf("rrflow-state binary missing on the host")
	}
	if err := pushFile(ctx, wat, ws, "/usr/local/bin/rrflow-state", "0755", string(bin)); err != nil {
		return fmt.Errorf("rrflow client not installed")
	}
	_, _ = wat.do(ctx, "exec", ws, "--", "ln", "-sf", "/usr/local/bin/rrflow-state", "/usr/local/bin/rrflow")
	if err := pushFile(ctx, wat, ws, "/etc/profile.d/shippin-rrflow.sh", "0644", rrflowProfile); err != nil {
		return err
	}
	_, _ = wat.do(ctx, "exec", ws, "--", "sh", "-c",
		`grep -q shippin-rrflow.sh /etc/bash.bashrc || echo '[ -r /etc/profile.d/shippin-rrflow.sh ] && . /etc/profile.d/shippin-rrflow.sh' >> /etc/bash.bashrc`)
	env := fmt.Sprintf("export RRFLOW_URL=http://%s:%d\nexport RRFLOW_TOKEN_FILE=\"$HOME/.shippin/rrflow.token\"\nexport RRFLOW_BINDING=%s\n", ip, c.Port, sb)
	uid := fmt.Sprint(c.WSUID)
	user := []string{"exec", ws, "--user", uid, "--group", uid, "--env", "HOME=/home/" + c.WSUser, "--"}
	write := `umask 077 && mkdir -p "$HOME/.shippin" && cat > "$HOME/.shippin/$N.tmp" && mv -f "$HOME/.shippin/$N.tmp" "$HOME/.shippin/$N"`
	for name, body := range map[string]string{"rrflow.token": tok + "\n", "rrflow.env": env} {
		args := append(append([]string{}, user[:len(user)-1]...), "--env", "N="+name, "--", "sh", "-c", write)
		if _, err := wat.inc(ctx, body, args...); err != nil {
			return fmt.Errorf("binding not written to the workspace")
		}
	}
	return nil
}

// UnbindRRFlow revokes the binding in the instance and removes the workspace's token (best effort
// on the workspace: it may be stopped or gone).
func (s *Server) UnbindRRFlow(ctx context.Context, t, si, sb string, r BindReq) error {
	at, err := s.servicePlace(ctx, t, r.Project, nil, r.Shared)
	if err != nil {
		return err
	}
	if _, err := at.do(ctx, "exec", ServiceInstance(si), "--env", "RRFLOW_DATA="+s.cfg.Services.RRFlow.DataDir, "--",
		"runuser", "-u", "rrflow", "--", "/usr/local/bin/rrflow-state", "unbind", sb); err != nil {
		return fmt.Errorf("binding not removed from the service")
	}
	if wat, err := s.placeOf(ctx, t, r.Workspace); err == nil && ValidUUID(r.Workspace) == nil {
		h := "/home/" + s.cfg.Services.RRFlow.WSUser + "/.shippin/"
		_, _ = wat.do(ctx, "exec", Instance(r.Workspace), "--", "rm", "-f", h+"rrflow.token", h+"rrflow.env")
	}
	return nil
}

// RRFlowExport reads one kind (sessions|memory|decisions) from the instance as JSON. Guest output
// is untrusted: it is size-bounded by place.inc and must parse as JSON.
// A shared (D36) instance exports namespace pj only.
func (s *Server) RRFlowExport(ctx context.Context, t, pj, si, kind string, limit int, shared bool) (json.RawMessage, error) {
	if kind != "sessions" && kind != "memory" && kind != "decisions" {
		return nil, fmt.Errorf("unknown kind")
	}
	if limit < 1 || limit > 500 {
		limit = 100
	}
	at, err := s.servicePlace(ctx, t, pj, nil, shared)
	if err != nil {
		return nil, err
	}
	args := []string{"exec", ServiceInstance(si), "--env", "RRFLOW_DATA=" + s.cfg.Services.RRFlow.DataDir,
		"--env", "RRFLOW_ORG=" + t, "--", "runuser", "-u", "rrflow", "--", "/usr/local/bin/rrflow-state", "export", kind, fmt.Sprint(limit)}
	if shared {
		args = append(args, pj)
	}
	out, err := at.do(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("rrflow/state unavailable")
	}
	if b := []byte(strings.TrimSpace(out)); json.Valid(b) {
		return b, nil
	}
	return nil, fmt.Errorf("rrflow/state output malformed")
}
