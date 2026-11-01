package hostd

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const vmsi = "019a0000-0000-7000-8000-0000000000a7"

func migrateServer(t *testing.T, enabled, retire bool) *Server {
	s := serviceServer(t)
	s.cfg.Services.RRFlow.Migrate = RRFlowMigrate{Enabled: enabled, RetireOld: retire}
	return s
}

// D36: a shared binding lives in the VM-wide inner project and names the project's namespace.
func TestSharedBindUsesNamespace(t *testing.T) {
	s := serviceServer(t)
	f := useFake(t, vmReplies(pjid)...)
	if err := s.BindRRFlow(context.Background(), tid, vmsi, sbid, BindReq{Project: pjid, Workspace: wsid, Shared: true}); err != nil {
		t.Fatal(err)
	}
	c := f.find("rrflow-state bind " + sbid + " " + wsid + " " + pjid)
	if c == nil || !strings.Contains(strings.Join(c, " "), "--project="+InnerProject+" ") {
		t.Fatalf("shared binding not recorded with its namespace in the VM-wide project: %v", c)
	}
	if f.find("pj-"+pjid, "si-"+vmsi) != nil {
		t.Fatal("shared instance addressed inside the project's inner project")
	}
}

func TestMigrateDisabledByDefault(t *testing.T) {
	f := useFake(t, vmReplies(pjid)...)
	if migrateServer(t, false, false).MigrateRRFlow(context.Background(), tid, pjid, siid, vmsi) == nil {
		t.Fatal("migrate ran with migrate.enabled off")
	}
	if len(f.calls) != 0 {
		t.Fatal("disabled migrate touched Incus")
	}
}

// Snapshot first, copy into the namespace, and never delete in the same run.
func TestMigrateSnapshotsCopiesNeverDeletes(t *testing.T) {
	s := migrateServer(t, true, true)
	f := useFake(t, append(vmReplies(pjid), fakeReply{match: "snapshot show", err: errors.New("not found")},
		fakeReply{match: "F=/var/lib/rrflow/memory.jsonl", out: `{"key":"k"}` + "\n"})...)
	if err := s.MigrateRRFlow(context.Background(), tid, pjid, siid, vmsi); err != nil {
		t.Fatal(err)
	}
	snap := f.find("--project=pj-"+pjid, "snapshot create", "si-"+siid, MigrateSnapshot(vmsi))
	push := f.find("--project="+InnerProject, "si-"+vmsi, "F=/var/lib/rrflow/ns/"+pjid+"/memory.jsonl.import")
	if snap == nil || push == nil || push[len(push)-1] != `{"key":"k"}`+"\n" {
		t.Fatal("no snapshot before the copy, or memory not copied into the namespace")
	}
	if idx(f, "snapshot create") > idx(f, "memory.jsonl.import") {
		t.Fatal("copied before the snapshot")
	}
	if f.find("delete", "si-"+siid) != nil {
		t.Fatal("source deleted in the migrate run")
	}
}

func TestRetireNeedsFlagSnapshotAndVerify(t *testing.T) {
	useFake(t, vmReplies(pjid)...)
	if migrateServer(t, true, false).RetireRRFlow(context.Background(), tid, pjid, siid, vmsi) == nil {
		t.Fatal("retired with retire_old off")
	}
	s := migrateServer(t, true, true)
	f := useFake(t, append(vmReplies(pjid), fakeReply{match: "snapshot show", err: errors.New("not found")})...)
	if s.RetireRRFlow(context.Background(), tid, pjid, siid, vmsi) == nil || f.find("delete") != nil {
		t.Fatal("retired without a pre-migrate snapshot")
	}
	f = useFake(t, append(vmReplies(pjid), fakeReply{match: "rrflow-state verify", err: errors.New("chain broken")})...)
	if s.RetireRRFlow(context.Background(), tid, pjid, siid, vmsi) == nil || f.find("delete") != nil {
		t.Fatal("retired although the target namespace does not verify")
	}
	f = useFake(t, vmReplies(pjid)...)
	if err := s.RetireRRFlow(context.Background(), tid, pjid, siid, vmsi); err != nil || f.find("--project=pj-"+pjid, "delete", "si-"+siid) == nil {
		t.Fatalf("verified retire did not delete the source: %v", err)
	}
}

func idx(f *fakeRunner, needle string) int {
	for i, c := range f.calls {
		if strings.Contains(strings.Join(c, " "), needle) {
			return i
		}
	}
	return -1
}
