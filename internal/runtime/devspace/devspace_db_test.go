package devspace_test

import (
	"context"
	"net/http"
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

type fakeOrgs struct{}

func (fakeOrgs) Org(_ context.Context, id string) (*devspace.Org, error) {
	if id != acme {
		return nil, nil
	}
	return &devspace.Org{ID: acme, Name: "Acme", Plan: "free", Status: "active", SSHIP: "203.0.113.7"}, nil
}

func serve(t *testing.T, hosts *fakeHosts, as mcpltest.As) *mcpltest.Harness {
	t.Helper()
	s := mcpl.Services{}
	mcpl.Provide[ports.Guests](s, hosts)
	mcpl.Provide[devspace.Orgs](s, fakeOrgs{})
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
	h.Call("GET", base+"/"+shop, "", http.StatusOK)
	h.Call("GET", base+"/"+blog, "", http.StatusNotFound)
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
