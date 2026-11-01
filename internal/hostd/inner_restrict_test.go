package hostd

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

const pjID = "019a0000-0000-7000-8000-00000000a001"
const wsID = "019a0000-0000-7000-8000-00000000b001"

func TestEnsureInnerProjectRestricted(t *testing.T) {
	f := useFake(t)
	at := place{t: tid, vm: true}.in(pjID)
	if err := (&Server{cfg: &Config{}}).ensureInnerProject(context.Background(), at, Plan{Instances: 4}); err != nil {
		t.Fatal(err)
	}
	c := f.find("project create pj-" + pjID)
	if c == nil || !slices.Contains(c, "features.profiles=true") || slices.Contains(c, "restricted=true") {
		t.Fatalf("create needs its own profiles (Incus refuses restricting a shared-profile project): %v", f.calls)
	}
	for _, want := range []string{"root disk path=/ pool=default", "eth0 nic network=incusbr0", "security.idmap.isolated=true"} {
		if f.find("--project=pj-"+pjID+" profile", want) == nil {
			t.Errorf("profile lacks %s: %v", want, f.calls)
		}
	}
	set := f.find("project set pj-" + pjID)
	for _, kv := range []string{"restricted=true", "restricted.containers.privilege=isolated",
		"restricted.devices.disk=managed", "restricted.devices.nic=managed", "restricted.containers.nesting=block",
		"limits.virtual-machines=0", "limits.instances=4"} {
		if !slices.Contains(set, kv) {
			t.Errorf("project set lacks %s", kv)
		}
	}
	if !slices.Contains(c, "--project=ws") || c[1] != "exec" || c[2] != "tvm-"+tid {
		t.Errorf("create must run inside the tenant VM from the legacy project: %v", c)
	}
}

func TestEnsureInnerProjectCleansUp(t *testing.T) {
	f := useFake(t, fakeReply{match: "profile device add default eth0", err: errors.New("no such network")})
	at := place{t: tid, vm: true}.in(pjID)
	if err := (&Server{cfg: &Config{}}).ensureInnerProject(context.Background(), at, Plan{}); err == nil {
		t.Fatal("want error")
	}
	if f.find("project delete pj-"+pjID) == nil || f.find("project set pj-"+pjID) != nil {
		t.Fatalf("half-made project must be deleted, never restricted: %v", f.calls)
	}
}

func TestInnerInitArgs(t *testing.T) {
	plan := Plan{WorkspaceCPU: 2, WorkspaceMem: "4GiB"}
	pj := place{t: tid, vm: true}.in(pjID)
	got := strings.Join(innerInitArgs(pj, plan, Size{}), " ")
	for _, want := range []string{"security.idmap.isolated=true", "limits.cpu=2", "limits.memory=4GiB"} {
		if !strings.Contains(got, want) {
			t.Errorf("pj init args %q lack %s", got, want)
		}
	}
	got = strings.Join(innerInitArgs(pj, plan, Size{CPU: 6, MemoryGiB: 8}), " ")
	if strings.Contains(got, "limits.cpu=2") || !strings.Contains(got, "limits.cpu=6") || !strings.Contains(got, "limits.memory=8GiB") {
		t.Errorf("template size must win: %q", got)
	}
	if a := innerInitArgs(place{t: tid, vm: true}, plan, Size{}); len(a) != 0 {
		t.Errorf("legacy ws keeps its profile: %v", a)
	}
	if a := innerInitArgs(place{t: tid}, plan, Size{}); len(a) != 0 {
		t.Errorf("host placement keeps its profile: %v", a)
	}
}

func TestMissingKeys(t *testing.T) {
	plan := Plan{WorkspaceCPU: 2, WorkspaceMem: "4GiB"}
	if kv := missingKeys(map[string]string{"security.idmap.isolated": "true", "limits.cpu": "1", "limits.memory": "1GiB"}, plan); len(kv) != 0 {
		t.Errorf("complete instance changed: %v", kv)
	}
	if kv := missingKeys(map[string]string{}, plan); len(kv) != 3 {
		t.Errorf("bare instance: %v", kv)
	}
}

func TestHardenProjectWaitsForRestart(t *testing.T) {
	list := `[{"name":"w-` + wsID + `","status":"Running","config":{}},{"name":"evil","status":"Running","config":{}}]`
	f := useFake(t, fakeReply{match: "/1.0/instances", out: list})
	pl := place{t: tid, vm: true}.in(pjID)
	h := (&Server{cfg: &Config{}}).hardenProject(context.Background(), pl, Plan{}, false)
	if h.Restricted || len(h.PendingRestart) != 1 || f.find("project set") != nil {
		t.Fatalf("must not restrict before restart: %+v", h)
	}
	if f.find("config set evil") != nil {
		t.Fatal("guest-reported non-workspace name was touched")
	}
	f = useFake(t, fakeReply{match: "/1.0/instances", out: list})
	h = (&Server{cfg: &Config{}}).hardenProject(context.Background(), pl, Plan{}, true)
	if !h.Restricted || f.find("restart w-"+wsID) == nil || f.find("project set pj-"+pjID, "restricted=true") == nil {
		t.Fatalf("restart path: %+v %v", h, f.calls)
	}
}

func TestInnerProjectOfUsesAPI(t *testing.T) {
	f := useFake(t, fakeReply{match: "all-projects=true", out: `[{"name":"w-` + wsID + `","project":"pj-` + pjID + `"},{"name":"w-` + wsID + `","project":"pj-evil"}]`})
	at := place{t: tid, vm: true}
	if got := (&Server{cfg: &Config{}}).innerProjectOf(context.Background(), at, wsID); got != pjID {
		t.Fatalf("got %q (%v)", got, f.calls)
	}
	if c := f.find("all-projects=true"); slices.Contains(c, "--project=ws") {
		t.Fatalf("the CLI refuses --project with all projects: %v", c)
	}
}
