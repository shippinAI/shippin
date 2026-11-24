package hostd

import (
	"context"
	"os"
	"strings"
	"testing"
)

const (
	pjid = "019a0000-0000-7000-8000-0000000000d4"
	siid = "019a0000-0000-7000-8000-0000000000e5"
	sbid = "019a0000-0000-7000-8000-0000000000f6"
)

func serviceServer(t *testing.T) *Server {
	s := testServer()
	bin := t.TempDir() + "/rrflow-state"
	if err := os.WriteFile(bin, []byte("BINARY"), 0o755); err != nil {
		t.Fatal(err)
	}
	s.cfg.Services.RRFlow = &RRFlowService{Image: "shippin/dev-debian13", Binary: bin, Port: 7710, CPU: 1, Memory: "512MiB",
		DataDir: "/var/lib/rrflow", WSUser: "dev", WSUID: 1000}
	return s
}

// vmReplies fakes a VM-backed tenant whose workspace lives in inner project pj-<project>.
func vmReplies(project string) []fakeReply {
	return []fakeReply{
		{match: "info w-" + wsid + " --project=t-", err: os.ErrNotExist},
		{match: "/1.0/projects/t-" + tid, out: `{"config":{"user.shippin.mode":"vm"}}`},
		{match: "/1.0/networks/n-" + tid, out: `{"config":{"user.shippin.inner_subnet":"10.128.9.0/24"}}`},
		{match: "/state", out: `{"network":{"eth0":{"addresses":[{"family":"inet","address":"10.128.9.7","scope":"global"}]}}}`},
		{match: "all-projects=true", out: `[{"name":"w-` + wsid + `","project":"pj-` + project + `"}]`},
	}
}

func TestServicesConfigCheck(t *testing.T) {
	s := serviceServer(t)
	if err := s.cfg.Services.check("shippin/"); err != nil {
		t.Fatal(err)
	}
	r := *s.cfg.Services.RRFlow
	r.DataDir = "/etc"
	if (ServicesConfig{RRFlow: &r}).check("shippin/") == nil {
		t.Fatal("data dir outside /var/lib accepted")
	}
}

func TestBindTokenOnlyOnStdin(t *testing.T) {
	s := serviceServer(t)
	f := useFake(t, vmReplies(pjid)...)
	if err := s.BindRRFlow(context.Background(), tid, siid, sbid, BindReq{Project: pjid, Workspace: wsid}); err != nil {
		t.Fatal(err)
	}
	bind := f.find("rrflow-state bind " + sbid + " " + wsid)
	if bind == nil {
		t.Fatal("binding not recorded in the instance")
	}
	tok := strings.TrimSpace(bind[len(bind)-1])
	if len(tok) < 40 {
		t.Fatalf("token too short: %d", len(tok))
	}
	inFile := false
	for _, c := range f.calls {
		if strings.Contains(strings.Join(c[:len(c)-1], " "), tok) {
			t.Fatal("binding token appeared in an argv")
		}
		inFile = inFile || (c[len(c)-1] == tok+"\n" && strings.Contains(strings.Join(c, " "), "N=rrflow.token"))
	}
	if !inFile || f.find("pj-"+pjid, "si-"+siid) == nil {
		t.Fatal("token file not written to the workspace, or instance not in the project")
	}
}

func TestBindRefusesOtherProjectAndLegacy(t *testing.T) {
	s := serviceServer(t)
	useFake(t, vmReplies("019a0000-0000-7000-8000-000000000999")...)
	if s.BindRRFlow(context.Background(), tid, siid, sbid, BindReq{Project: pjid, Workspace: wsid}) == nil {
		t.Fatal("bound a workspace of another project")
	}
	useFake(t) // legacy tenant: no VM mode
	if _, err := s.EnsureRRFlow(context.Background(), tid, siid, ServiceReq{Project: pjid, Plan: "founder"}); err == nil {
		t.Fatal("legacy tenant got a service instance")
	}
	if _, err := s.RRFlowExport(context.Background(), tid, pjid, siid, "passwd", 10, false); err == nil {
		t.Fatal("unknown export kind accepted")
	}
}
