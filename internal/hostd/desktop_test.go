package hostd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"shippin.cloud/console/internal/apps"
)

const desktopTenant = "00000000-0000-4000-8000-000000000001"

func useVMFake(t *testing.T, replies ...fakeReply) *fakeRunner {
	t.Helper()
	f := useFake(t, replies...)
	old := vmRunner
	vmRunner = func(ctx context.Context, stdin string, args ...string) (string, error) {
		return runner(ctx, stdin, "incus", args...)
	}
	t.Cleanup(func() { vmRunner = old })
	return f
}

func TestClassifyDesktopTarget(t *testing.T) {
	id := desktopTenant
	if err := classifyDesktopTarget("platform", id, TenantVM(id), "virtual-machine"); err != nil {
		t.Fatal(err)
	}
	if err := classifyDesktopTarget("t-"+id, id, TenantVM(id), "virtual-machine"); err == nil || !strings.Contains(err.Error(), "control-plane") {
		t.Fatalf("control plane: %v", err)
	}
	if err := classifyDesktopTarget("platform", id, TenantVM(id), "container"); err == nil || !strings.Contains(err.Error(), "container") {
		t.Fatalf("container: %v", err)
	}
	if err := classifyDesktopTarget("platform", id, "other", "virtual-machine"); err == nil || !strings.Contains(err.Error(), "non-tenant-vm") {
		t.Fatalf("name: %v", err)
	}
}

func TestDesktopPrepareRejectsParameters(t *testing.T) {
	f := useVMFake(t)
	mux := testServer().Routes()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tenants/"+desktopTenant+"/desktop/prepare", strings.NewReader(`{"cmd":"rm -rf /"}`))
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code %d body %s", rec.Code, rec.Body.String())
	}
	if f.find("incus") != nil || f.find("bash") != nil {
		t.Fatal("a parameter body must not reach incus")
	}
}

func TestDesktopPrepareRefusesContainer(t *testing.T) {
	f := useVMFake(t, fakeReply{match: "query /1.0/instances/tvm-", out: `{"name":"` + TenantVM(desktopTenant) + `","type":"container"}`})
	rec := httptest.NewRecorder()
	testServer().Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/tenants/"+desktopTenant+"/desktop/prepare", nil))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "container") {
		t.Fatalf("code %d body %s", rec.Code, rec.Body.String())
	}
	if f.find("bash") != nil {
		t.Fatal("container must not be exec'd")
	}
}

func TestDesktopPrepareRefusesControlPlane(t *testing.T) {
	f := useVMFake(t)
	s := testServer()
	s.cfg.Platform = Project(desktopTenant)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/tenants/"+desktopTenant+"/desktop/prepare", nil))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "control-plane") {
		t.Fatalf("code %d body %s", rec.Code, rec.Body.String())
	}
	if f.find("query") != nil || f.find("bash") != nil {
		t.Fatal("control-plane refusal must happen before any incus call")
	}
}

func TestDesktopPrepareArgvIsFixed(t *testing.T) {
	vm := `{"name":"` + TenantVM(desktopTenant) + `","type":"virtual-machine"}`
	f := useVMFake(t, fakeReply{match: "query /1.0/instances/tvm-", out: vm})
	rec := httptest.NewRecorder()
	testServer().Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/tenants/"+desktopTenant+"/desktop/prepare", strings.NewReader("\n")))
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d body %s", rec.Code, rec.Body.String())
	}
	prereq := f.find("bash", "-s")
	if prereq == nil {
		t.Fatal("prereq exec missing")
	}
	joined := strings.Join(prereq, "\n")
	want := "incus exec " + TenantVM(desktopTenant) + " --project=" + Project(desktopTenant) + " --force-noninteractive -- bash -s --"
	if !strings.Contains(strings.Join(prereq[:len(prereq)-1], " "), strings.TrimPrefix(want, "incus ")) && !strings.Contains(strings.Join(prereq, " "), "exec "+TenantVM(desktopTenant)) {
		t.Fatalf("argv %q", prereq)
	}
	stdin := prereq[len(prereq)-1]
	if !strings.Contains(stdin, "xfce4") || strings.Contains(stdin, "claude-desktop") || strings.Contains(stdin, "chatgpt.com") || strings.Contains(stdin, "rm -rf") {
		t.Fatal("prereq stdin is not the embedded desktop script")
	}
	helper := f.find("cat > /usr/local/bin/shippin")
	if helper == nil {
		t.Fatal("helper install missing")
	}
	if !strings.Contains(helper[len(helper)-1], apps.ClaudeKeyFingerprint) {
		t.Fatal("helper stdin is not the catalog script")
	}
	if strings.Contains(joined, `{"cmd"`) {
		t.Fatal("request body leaked into argv")
	}
}

func TestDesktopBridgeIsLoopbackOnly(t *testing.T) {
	if strings.Contains(desktopBridgeScript, "0.0.0.0") || !strings.Contains(desktopBridgeScript, "127.0.0.1") || !strings.Contains(desktopBridgeScript, "3389") {
		t.Fatal(desktopBridgeScript)
	}
	args := strings.Join(desktopBridgeArgs(desktopTenant), " ")
	if !strings.Contains(args, "exec "+TenantVM(desktopTenant)) || !strings.Contains(args, "--project="+Project(desktopTenant)) {
		t.Fatal(args)
	}
	if strings.Contains(args, "0.0.0.0") {
		t.Fatal(args)
	}
}

func TestNoWorkspaceDesktopRoute(t *testing.T) {
	f := useVMFake(t)
	rec := httptest.NewRecorder()
	testServer().Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/tenants/"+desktopTenant+"/workspaces/"+desktopTenant+"/desktop", strings.NewReader(`{"cmd":"rm -rf /"}`)))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "unknown action") {
		t.Fatalf("workspace desktop route: %d %s", rec.Code, rec.Body.String())
	}
	if f.find("incus") != nil {
		t.Fatal("a workspace path must not reach the desktop exec")
	}
}

func TestAppsStatusDropsSecrets(t *testing.T) {
	guest := `{"packages":{"claude-desktop":"3.6556.1","claude-code":"SECRET TOKEN","chatgpt":""},"users":[{"user":"operator","uid":10000,"claude":true,"claude_desktop":false,"codex":false,"chatgpt":false},{"user":"root","uid":0,"claude":true,"claude_desktop":false,"codex":false,"chatgpt":false}],"xrdp_loopback":true,"grok_desktop":false,"kvm":true,"signed_in":"yes"}`
	vm := `{"name":"` + TenantVM(desktopTenant) + `","type":"virtual-machine"}`
	useVMFake(t,
		fakeReply{match: "query /1.0/instances/tvm-", out: vm},
		fakeReply{match: "bash -s", out: guest},
	)
	rec := httptest.NewRecorder()
	testServer().Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tenants/"+desktopTenant+"/apps/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("code %d %s", rec.Code, rec.Body.String())
	}
	var st apps.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if st.SignedIn != "unknown" || strings.Contains(rec.Body.String(), "SECRET") || strings.Contains(rec.Body.String(), `"user":"root"`) {
		t.Fatalf("guest output not normalized: %s", rec.Body.String())
	}
	if st.Packages["claude-desktop"] != "3.6556.1" || st.Packages["claude-code"] != "" {
		t.Fatalf("packages %+v", st.Packages)
	}
	if len(st.Users) != 1 || st.Users[0].User != "operator" || !st.Users[0].Claude {
		t.Fatalf("users %+v", st.Users)
	}
}

func TestDesktopTunnelRefusesBeforeHijack(t *testing.T) {
	old := desktopCmd
	desktopCmd = func(context.Context, []string) *exec.Cmd {
		t.Fatal("desktop command must not run when the target is refused")
		return nil
	}
	t.Cleanup(func() { desktopCmd = old })
	f := useVMFake(t, fakeReply{match: "query /1.0/instances/tvm-", out: `{"name":"` + TenantVM(desktopTenant) + `","type":"container"}`})
	req := httptest.NewRequest(http.MethodGet, "/tenants/"+desktopTenant+"/desktop", nil)
	req.Header.Set("Upgrade", DesktopProto)
	rec := httptest.NewRecorder()
	testServer().Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("code %d %s", rec.Code, rec.Body.String())
	}
	if f.find("bash") != nil {
		t.Fatal("refused tunnel still exec'd")
	}
}
