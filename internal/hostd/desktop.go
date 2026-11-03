package hostd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	_ "embed"
	"shippin.cloud/console/internal/apps"
)

//go:embed desktop_prereq.sh
var desktopPrereqScript string

//go:embed desktop_status.sh
var desktopStatusScript string

// DesktopProto is the Upgrade token for the byte pipe to xrdp on the tenant VM.
const DesktopProto = "shippin-desktop"

// desktopBridgeScript joins stdin/stdout to xrdp on the VM loopback. The address and port
// are fixed; the API cannot choose them.
const desktopBridgeScript = `exec 3<>/dev/tcp/127.0.0.1/3389 || exit 1; cat <&3 & cat >&3; kill $! 2>/dev/null`

// vmRunner is the exec seam for scripts inside the tenant VM. Tests replace it.
var vmRunner = func(ctx context.Context, stdin string, args ...string) (string, error) {
	return runFor(ctx, 20*time.Minute, stdin, "incus", args...)
}

// desktopCmd is the seam for the long-running RDP pipe.
var desktopCmd = func(ctx context.Context, args []string) *exec.Cmd {
	return exec.CommandContext(ctx, "incus", args...)
}

// classifyDesktopTarget refuses anything that is not this tenant's KVM VM.
// A container (including a workspace) and the control-plane project are both refused.
func classifyDesktopTarget(platform, tenant, name, typ string) error {
	if platform != "" && Project(tenant) == platform {
		return fmt.Errorf("refusing control-plane project")
	}
	if name != TenantVM(tenant) {
		return fmt.Errorf("refusing non-tenant-vm target")
	}
	if typ != "virtual-machine" {
		return fmt.Errorf("refusing container")
	}
	return nil
}

func (s *Server) requireTenantVM(ctx context.Context, tenant string) error {
	if err := classifyDesktopTarget(s.cfg.Platform, tenant, TenantVM(tenant), "virtual-machine"); err != nil {
		return err
	}
	var inst struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := query(ctx, fmt.Sprintf("/1.0/instances/%s?project=%s", TenantVM(tenant), Project(tenant)), &inst); err != nil {
		return fmt.Errorf("tenant VM not found")
	}
	return classifyDesktopTarget(s.cfg.Platform, tenant, inst.Name, inst.Type)
}

// desktopBashArgs is the only argv used to run a script inside the tenant VM.
func desktopBashArgs(tenant string) []string {
	return []string{"exec", TenantVM(tenant), "--project=" + Project(tenant), "--force-noninteractive", "--", "bash", "-s", "--"}
}

// desktopBridgeArgs is the only argv used to open the RDP pipe.
func desktopBridgeArgs(tenant string) []string {
	return []string{"exec", TenantVM(tenant), "--project=" + Project(tenant), "--force-noninteractive", "--", "bash", "-c", desktopBridgeScript}
}

func rejectParams(r *http.Request) error {
	b, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(b)) > 0 {
		return fmt.Errorf("this action takes no parameters")
	}
	return nil
}

func desktopCode(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case strings.Contains(err.Error(), "no parameters"):
		return http.StatusBadRequest
	case strings.Contains(err.Error(), "refusing"), strings.Contains(err.Error(), "not found"):
		return http.StatusConflict
	default:
		return http.StatusBadGateway
	}
}

func (s *Server) desktopPrepare(w http.ResponseWriter, r *http.Request) {
	t := r.PathValue("t")
	if err := rejectParams(r); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.requireTenantVM(r.Context(), t); err != nil {
		fail(w, desktopCode(err), err.Error())
		return
	}
	if _, err := vmRunner(r.Context(), desktopPrereqScript, desktopBashArgs(t)...); err != nil {
		fail(w, http.StatusBadGateway, "desktop prerequisites failed")
		return
	}
	// The helper is a fixed script from this binary, not from the request.
	install := "cat > /usr/local/bin/shippin && chmod 755 /usr/local/bin/shippin"
	args := []string{"exec", TenantVM(t), "--project=" + Project(t), "--force-noninteractive", "--", "bash", "-c", install}
	if _, err := vmRunner(r.Context(), apps.HelperScript(), args...); err != nil {
		fail(w, http.StatusBadGateway, "desktop helper install failed")
		return
	}
	reply(w, map[string]string{"status": "ok"})
}

func (s *Server) appsStatus(w http.ResponseWriter, r *http.Request) {
	t := r.PathValue("t")
	st, err := s.ReadAppsStatus(r.Context(), t)
	if err != nil {
		fail(w, desktopCode(err), err.Error())
		return
	}
	reply(w, st)
}

// ReadAppsStatus runs the fixed status script in the tenant VM and keeps only typed fields.
func (s *Server) ReadAppsStatus(ctx context.Context, tenant string) (*apps.Status, error) {
	if err := s.requireTenantVM(ctx, tenant); err != nil {
		return nil, err
	}
	out, err := vmRunner(ctx, desktopStatusScript, desktopBashArgs(tenant)...)
	if err != nil {
		return nil, fmt.Errorf("status failed")
	}
	if len(out) > 64<<10 {
		return nil, fmt.Errorf("status failed")
	}
	var st apps.Status
	if json.Unmarshal([]byte(out), &st) != nil {
		return nil, fmt.Errorf("status failed")
	}
	apps.Normalize(&st)
	return &st, nil
}

func (s *Server) desktopTunnel(w http.ResponseWriter, r *http.Request) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), DesktopProto) {
		fail(w, http.StatusUpgradeRequired, "desktop needs Upgrade: "+DesktopProto)
		return
	}
	t := r.PathValue("t")
	if err := s.requireTenantVM(r.Context(), t); err != nil {
		fail(w, desktopCode(err), err.Error())
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		fail(w, http.StatusInternalServerError, "desktop unsupported")
		return
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Time{})
	_, _ = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: " + DesktopProto + "\r\n\r\n")
	if rw.Flush() != nil {
		return
	}
	cmd := desktopCmd(context.Background(), desktopBridgeArgs(t))
	cmd.Stdin, cmd.Stdout = rw.Reader, conn
	_ = cmd.Run()
}
