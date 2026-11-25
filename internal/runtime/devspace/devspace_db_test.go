package devspace_test

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/mcpl/mcpltest"
	"shippin.cloud/console/internal/platform/config"
	"shippin.cloud/console/internal/ports"
	"shippin.cloud/console/internal/runtime/devspace"
)

const (
	acme   = "10000000-0000-4000-8000-000000000001"
	globex = "10000000-0000-4000-8000-000000000002"
	shop   = "20000000-0000-4000-8000-00000000000a"
	blog   = "20000000-0000-4000-8000-00000000000b"
	theirs = "20000000-0000-4000-8000-00000000000c"
)

// fakeHosts answers for the hosts: what each organization's guests are, or that they could not be read.
type fakeHosts struct {
	guests map[string]ports.Guest
	down   bool
}

func (f *fakeHosts) Inventory(context.Context, string) (map[string]ports.Guest, bool) {
	return f.guests, !f.down
}

func (f *fakeHosts) Limits(context.Context, string) ports.GuestLimits {
	return ports.GuestLimits{CPU: 2, Memory: "4GiB"}
}

type fakeOrgs struct{ status string }

func (f *fakeOrgs) Org(_ context.Context, id string) (*devspace.Org, error) {
	if id != acme {
		return nil, nil
	}
	status := f.status
	if status == "" {
		status = "active"
	}
	return &devspace.Org{ID: acme, Name: "Acme", Plan: "free", Status: status, SSHIP: "203.0.113.7"}, nil
}

// fakeQueue records the work it is given and refuses a space that already has some.
type fakeQueue struct {
	work  []devspace.Work
	by    []string
	busy  map[string]bool
	woken int
}

func (q *fakeQueue) Apply(_ context.Context, tx mcpl.DB, w devspace.Devspace, work devspace.Work, by string) (devspace.Job, error) {
	if tx == nil {
		panic("no transaction")
	}
	if q.busy[w.ID] {
		return devspace.Job{}, devspace.ErrBusy
	}
	if q.busy == nil {
		q.busy = map[string]bool{}
	}
	q.busy[w.ID] = true
	q.work, q.by = append(q.work, work), append(q.by, by)
	return devspace.Job{ID: "job-" + work.Action, TenantID: w.TenantID, Kind: "workspace.action",
		Target: devspace.JobTarget{Type: "workspace", ID: w.ID}, Status: "queued", CreatedBy: by}, nil
}

func (q *fakeQueue) Wake() { q.woken++ }

// orgs and queue are the fakes of the harness serve last built.
var (
	orgs  = &fakeOrgs{}
	queue = &fakeQueue{}
)

func serve(t *testing.T, hosts *fakeHosts, as mcpltest.As) *mcpltest.Harness {
	t.Helper()
	s := mcpl.Services{}
	mcpl.Provide[ports.Guests](s, hosts)
	orgs, queue = &fakeOrgs{}, &fakeQueue{}
	mcpl.Provide[devspace.Orgs](s, orgs)
	mcpl.Provide[devspace.Queue](s, queue)
	mcpl.Provide[config.Workspaces](s, config.Workspaces{User: "dev", CodeDir: "/home/dev/code", IDEPort: 8443})
	mcpl.Provide[config.Incus](s, config.Incus{HostLabel: "lab"})
	as.Actor, as.Tenant, as.Services = "ada", acme, s
	h := mcpltest.Serve(t, actions.All, "shippin.runtime.devspace.", as)
	ctx := t.Context()
	if _, err := h.Pool.Exec(ctx, `INSERT INTO tenants(id,idp_org_id,name,plan,status) VALUES($1,'org-acme','Acme','free','active'),($2,'org-globex','Globex','free','active')`, acme, globex); err != nil {
		t.Fatal(err)
	}
	for _, w := range []devspace.Devspace{
		{ID: shop, TenantID: acme, Name: "shop", CreatedBy: "ada", Host: "shop.example.test"},
		{ID: blog, TenantID: acme, Name: "blog", CreatedBy: "ada", Host: "blog.example.test"},
		{ID: theirs, TenantID: globex, Name: "theirs", CreatedBy: "bob", Host: "theirs.example.test"},
	} {
		if err := devspace.On(h.Pool).Create(ctx, &w, 22000, 22999); err != nil {
			t.Fatal(err)
		}
		if err := devspace.On(h.Pool).SetState(ctx, w.ID, "running", "", ""); err != nil {
			t.Fatal(err)
		}
	}
	return h
}

func names(out map[string]any) []string {
	var got []string
	for _, it := range out["items"].([]any) {
		got = append(got, it.(map[string]any)["name"].(string))
	}
	return got
}

// A member reads their organization's spaces in the state the hosts report, with how to reach each,
// and never another organization's.
func TestListAndGetAsAMember(t *testing.T) {
	hosts := &fakeHosts{guests: map[string]ports.Guest{
		devspace.GuestName(shop): {Status: "Running", IPv4: "10.0.0.5", MemoryBytes: 1 << 20},
		devspace.GuestName(blog): {Status: "Stopped"},
	}}
	h := serve(t, hosts, mcpltest.As{Perms: func(role string) bool { return role == "manage" }})
	base := "/v1/tenants/" + acme + "/runtime/devspaces"

	out := h.Call("GET", base, "", http.StatusOK)
	if got := names(out); len(got) != 2 || got[0] != "shop" || got[1] != "blog" {
		t.Fatalf("listed %v, want shop and blog only", got)
	}
	if out["can_manage"] != true || out["tenant_status"] != "active" {
		t.Errorf("can_manage=%v tenant_status=%v", out["can_manage"], out["tenant_status"])
	}
	if plan := out["plan"].(map[string]any); plan["name"] != "free" || plan["limits"].(map[string]any)["workspace_cpu"] != float64(2) {
		t.Errorf("plan: %v", plan)
	}

	one := h.Call("GET", base+"/"+shop, "", http.StatusOK)
	if one["state"] != "running" || one["observation"] != "observed" || one["ipv4"] != "10.0.0.5" || one["region"] != "lab" {
		t.Errorf("shop: state=%v observation=%v ipv4=%v region=%v", one["state"], one["observation"], one["ipv4"], one["region"])
	}
	if c := one["connect"].(map[string]any); c["host"] != "203.0.113.7" || c["user"] != "dev" {
		t.Errorf("connect: %v", c)
	}
	if steps, ok := one["steps"].([]any); !ok || len(steps) != 0 {
		t.Errorf("a running space has steps %v", one["steps"])
	}
	if got := offered(one); got != "start:false stop:true restart:true delete:true" {
		t.Errorf("a manager is offered %q on a running space", got)
	}
	if got := offered(h.Call("GET", base+"/"+blog, "", http.StatusOK)); got != "start:true stop:false restart:false delete:true" {
		t.Errorf("a manager is offered %q on a stopped space", got)
	}
	if s := h.Call("GET", base+"/"+blog, "", http.StatusOK)["state"]; s != "stopped" {
		t.Errorf("blog is %v, want stopped", s)
	}
	h.Call("GET", base+"/"+theirs, "", http.StatusNotFound)
	if len(h.Audits) != 0 || len(h.Events) != 0 {
		t.Errorf("a read was audited or published: %v %v", h.Audits, h.Events)
	}
}

// A person holding a support grant for one space is answered with that space and no other.
func TestListAndGetUnderAGrantForOneSpace(t *testing.T) {
	h := serve(t, &fakeHosts{guests: map[string]ports.Guest{}}, mcpltest.As{Sees: func(id string) bool { return id == shop }})
	base := "/v1/tenants/" + acme + "/runtime/devspaces"
	out := h.Call("GET", base, "", http.StatusOK)
	if got := names(out); len(got) != 1 || got[0] != "shop" {
		t.Fatalf("listed %v, want shop only", got)
	}
	if out["can_manage"] != false {
		t.Errorf("a grant for one space manages the organization's spaces")
	}
	if got := offered(h.Call("GET", base+"/"+shop, "", http.StatusOK)); got != "start:false stop:false restart:false delete:false" {
		t.Errorf("someone who does not manage the space is offered %q", got)
	}
	h.Call("GET", base+"/"+blog, "", http.StatusNotFound)
}

// offered is the actions a read offers, each with whether it can be called now.
func offered(view map[string]any) string {
	var out []string
	for _, a := range view["actions"].([]any) {
		o := a.(map[string]any)
		id := o["id"].(string)
		if o["enabled"] != true && o["why"] == nil {
			out = append(out, "no reason given")
		}
		out = append(out, fmt.Sprintf("%s:%v", id[strings.LastIndex(id, ".")+1:], o["enabled"]))
	}
	return strings.Join(out, " ")
}

// A lifecycle action is accepted as durable work on the action's own transaction, audited and
// published, and the workers are told once it is committed. A second one while the first is in
// progress is refused.
func TestLifecycleActionsAreAcceptedAsWork(t *testing.T) {
	h := serve(t, &fakeHosts{}, mcpltest.As{})
	h.React(actions.Reactions)
	base := "/v1/tenants/" + acme + "/runtime/devspaces/"
	out := h.Call("POST", base+shop+"/stop", "", http.StatusAccepted)
	if out["id"] != shop || out["state"] != "stopping" || out["job"].(map[string]any)["id"] != "job-stop" {
		t.Fatalf("accepted: %v", out)
	}
	if len(queue.work) != 1 || queue.work[0].Action != "stop" || queue.work[0].Grant != "" || queue.by[0] != "ada" {
		t.Fatalf("recorded work %+v by %v", queue.work, queue.by)
	}
	if len(h.Audits) != 1 || !strings.HasPrefix(h.Audits[0], "shippin.runtime.devspace.stop ") || !strings.Contains(h.Audits[0], shop) {
		t.Errorf("audit: %v", h.Audits)
	}
	if len(h.Events) != 1 || h.Events[0].ID != "shippin.runtime.devspace.stopped" || h.Events[0].Subject["devspaceId"] != shop {
		t.Errorf("events: %+v", h.Events)
	}
	if queue.woken != 1 {
		t.Errorf("the workers were woken %d times, want once", queue.woken)
	}
	h.Call("POST", base+shop+"/start", "", http.StatusConflict)
	h.Call("POST", base+theirs+"/start", "", http.StatusNotFound)
	h.Call("POST", base+"20000000-0000-4000-8000-0000000000ff/start", "", http.StatusNotFound)
	if len(queue.work) != 1 || len(h.Audits) != 1 {
		t.Errorf("a refused action recorded work or an audit entry: %+v %v", queue.work, h.Audits)
	}
	h.Call("POST", base+blog+"/restart", "", http.StatusAccepted)
	if out := h.Call("DELETE", base+"20000000-0000-4000-8000-0000000000ff", "", http.StatusNotFound); out["message"] == nil {
		t.Errorf("a refusal without a message: %v", out)
	}
}

// Work accepted under a support grant keeps the grant, so it is checked again when the work runs,
// and a delete under a grant needs the name of the space typed. A member deletes without it.
func TestLifecycleUnderAGrant(t *testing.T) {
	h := serve(t, &fakeHosts{}, mcpltest.As{Grant: "grant-7", Sees: func(id string) bool { return id == shop }})
	base := "/v1/tenants/" + acme + "/runtime/devspaces/"
	h.Call("DELETE", base+shop, "", http.StatusBadRequest)
	h.Call("DELETE", base+shop, `{"confirm":"blog"}`, http.StatusBadRequest)
	if len(queue.work) != 0 {
		t.Fatalf("an unconfirmed delete recorded work: %+v", queue.work)
	}
	h.Call("DELETE", base+shop, `{"confirm":"shop"}`, http.StatusAccepted)
	if len(queue.work) != 1 || queue.work[0].Action != "delete" || queue.work[0].Grant != "grant-7" {
		t.Fatalf("recorded work %+v, want the delete with its grant", queue.work)
	}

	member := serve(t, &fakeHosts{}, mcpltest.As{})
	member.Call("DELETE", base+shop, "", http.StatusAccepted)
	if len(queue.work) != 1 || queue.work[0].Grant != "" {
		t.Fatalf("a delete by a member: %+v", queue.work)
	}
}

// A suspended organization has no space started, and nothing is done in one being deleted.
func TestLifecycleFollowsTheOrganization(t *testing.T) {
	h := serve(t, &fakeHosts{}, mcpltest.As{})
	base := "/v1/tenants/" + acme + "/runtime/devspaces/"
	orgs.status = "suspended"
	h.Call("POST", base+shop+"/start", "", http.StatusConflict)
	h.Call("POST", base+shop+"/restart", "", http.StatusConflict)
	h.Call("POST", base+shop+"/stop", "", http.StatusAccepted)
	orgs.status = "deleting"
	h.Call("POST", base+blog+"/stop", "", http.StatusConflict)
	h.Call("DELETE", base+blog, "", http.StatusConflict)
	if len(queue.work) != 1 || queue.work[0].Action != "stop" {
		t.Fatalf("recorded work %+v, want the one stop", queue.work)
	}
}

// When the hosts cannot be read, a space's state is unknown: the stored state is kept beside it and
// never shown as live. When they answer and do not list it, it is missing.
func TestStateIsNeverGuessed(t *testing.T) {
	hosts := &fakeHosts{down: true}
	h := serve(t, hosts, mcpltest.As{})
	path := "/v1/tenants/" + acme + "/runtime/devspaces/" + shop
	one := h.Call("GET", path, "", http.StatusOK)
	if one["state"] != "unknown" || one["recorded_state"] != "running" || one["observation"] != "unavailable" {
		t.Errorf("hosts down: state=%v recorded=%v observation=%v", one["state"], one["recorded_state"], one["observation"])
	}
	hosts.down = false
	one = h.Call("GET", path, "", http.StatusOK)
	if one["state"] != "unknown" || one["observation"] != "missing" {
		t.Errorf("not listed: state=%v observation=%v", one["state"], one["observation"])
	}
}
