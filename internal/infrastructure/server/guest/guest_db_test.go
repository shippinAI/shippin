package guest_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/infrastructure/servers"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/mcpl/mcpltest"
	"shippin.cloud/console/internal/ports"
)

const base = "/v1/operator/infrastructure/servers"

// fakeHV is a hypervisor that remembers what it was asked to do.
type fakeHV struct {
	guests  map[string]hv.Guest
	calls   []string
	created hv.GuestSpec
}

func (f *fakeHV) Kind() string                              { return "incus" }
func (f *fakeHV) Health(context.Context) (hv.Health, error) { return hv.Health{Online: true}, nil }
func (f *fakeHV) Guests(context.Context) ([]hv.Guest, error) {
	out := []hv.Guest{}
	for _, id := range []string{"web", "panel-vm"} {
		if g, ok := f.guests[id]; ok {
			out = append(out, g)
		}
	}
	return out, nil
}
func (f *fakeHV) Guest(_ context.Context, id string) (hv.Guest, error) {
	g, ok := f.guests[id]
	if !ok {
		return hv.Guest{}, hv.ErrGuestNotFound
	}
	return g, nil
}
func (f *fakeHV) set(verb, id, status string) (hv.Guest, error) {
	f.calls = append(f.calls, verb+" "+id)
	g := f.guests[id]
	g.Status = status
	f.guests[id] = g
	return g, nil
}
func (f *fakeHV) Start(_ context.Context, id string) (hv.Guest, error) {
	return f.set("start", id, "running")
}
func (f *fakeHV) Stop(_ context.Context, id string) (hv.Guest, error) {
	return f.set("stop", id, "stopped")
}
func (f *fakeHV) Shutdown(_ context.Context, id string) (hv.Guest, error) {
	return f.set("shutdown", id, "stopped")
}
func (f *fakeHV) Reboot(_ context.Context, id string) (hv.Guest, error) {
	return f.set("reboot", id, "running")
}
func (f *fakeHV) Snapshot(context.Context, string, string) (hv.Guest, error) {
	return hv.Guest{}, hv.ErrUnsupported
}
func (f *fakeHV) Storage(context.Context) ([]hv.Volume, error)   { return nil, nil }
func (f *fakeHV) Networks(context.Context) ([]hv.Network, error) { return nil, nil }
func (f *fakeHV) Images(context.Context) ([]hv.Image, error)     { return nil, nil }
func (f *fakeHV) CreateGuest(_ context.Context, spec hv.GuestSpec) (hv.Guest, error) {
	f.calls, f.created = append(f.calls, "create "+spec.Name), spec
	g := hv.Guest{ID: spec.Name, Name: spec.Name, Kind: spec.Kind, Status: "stopped"}
	f.guests[spec.Name] = g
	return g, nil
}
func (f *fakeHV) DeleteGuest(_ context.Context, id string) error {
	f.calls = append(f.calls, "delete "+id)
	delete(f.guests, id)
	return nil
}

// fakeHosts is ports.Hypervisors with one server, s1, and a record of every tracked change.
type fakeHosts struct {
	hv       *fakeHV
	tracked  []servers.Operation
	recent   map[string]string
	noRecord bool
	review   bool
	reviewed []ports.ReviewedPower
	// alias is a second id the one server answers to, for a test that needs it to be a stored row's.
	alias string
}

func (f *fakeHosts) Servers(context.Context) ([]servers.Endpoint, error) {
	return []servers.Endpoint{{ID: "s1", Kind: "incus", BaseURL: "https://host.example:8443", HasSecret: true}}, nil
}
func (f *fakeHosts) Server(_ context.Context, id string) (servers.Endpoint, error) {
	if id != "s1" && (f.alias == "" || id != f.alias) {
		return servers.Endpoint{}, servers.ErrNotFound
	}
	return servers.Endpoint{ID: id, Kind: "incus", BaseURL: "https://host.example:8443", HasSecret: true}, nil
}
func (f *fakeHosts) Open(servers.Endpoint) (hv.Driver, error) { return f.hv, nil }
func (f *fakeHosts) Track(ctx context.Context, op servers.Operation, change func(context.Context) (hv.Guest, error)) (hv.Guest, error) {
	if f.noRecord {
		return hv.Guest{}, errors.New("the change could not be recorded, so it was not made")
	}
	f.tracked = append(f.tracked, op)
	return change(ctx)
}
func (f *fakeHosts) RecentOutcomes(context.Context, string) map[string]string { return f.recent }
func (f *fakeHosts) ImageSource(_ context.Context, _, image string) (string, string, string, bool) {
	if alias, ok := strings.CutPrefix(image, "images:"); ok {
		return alias, "https://images.example", "simplestreams", true
	}
	return "", "", "", false
}
func (f *fakeHosts) OutsideReview(context.Context) bool { return f.review }
func (f *fakeHosts) ReviewedPower(_ context.Context, p ports.ReviewedPower) (hv.Guest, error) {
	f.reviewed = append(f.reviewed, p)
	return f.hv.guests[p.Guest], nil
}

func serve(t *testing.T, may bool) (*mcpltest.Harness, *fakeHosts) {
	t.Setenv("SHIPPIN_SELF_GUEST", "panel-vm")
	hosts := &fakeHosts{hv: &fakeHV{guests: map[string]hv.Guest{
		"web":      {ID: "web", Name: "web", Kind: "ct", Status: "running"},
		"panel-vm": {ID: "panel-vm", Name: "panel-vm", Kind: "vm", Status: "running"},
	}}}
	s := mcpl.Services{}
	mcpl.Provide[ports.Hypervisors](s, hosts)
	h := mcpltest.Serve(t, actions.All, "shippin.infrastructure.server.", mcpltest.As{Actor: "ada", Services: s,
		Perms: func(perm string) bool { return may && perm == "infra.self" }})
	return h, hosts
}

func TestServersAndGuestsAreRead(t *testing.T) {
	h, hosts := serve(t, false)
	list := h.Call("GET", base, "", 200)["items"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["id"] != "s1" || list[0].(map[string]any)["has_secret"] != true {
		t.Fatalf("servers: %v", list)
	}
	if got := h.Call("GET", base+"/s1", "", 200); got["kind"] != "incus" {
		t.Fatalf("server: %v", got)
	}
	h.Call("GET", base+"/nope", "", 404)
	h.Call("GET", base+"/nope/guests", "", 404)

	// A change the panel just made outranks the hypervisor's lagging inventory, and the panel's own
	// guest is marked.
	hosts.recent = map[string]string{"web": "stopped"}
	guests := h.Call("GET", base+"/s1/guests", "", 200)["items"].([]any)
	web, self := guests[0].(map[string]any), guests[1].(map[string]any)
	if web["status"] != "stopped" || web["self"] == true || self["self"] != true {
		t.Fatalf("guests: %v", guests)
	}
	if got := h.Call("GET", base+"/s1/guests/panel-vm", "", 200); got["self"] != true {
		t.Fatalf("guest: %v", got)
	}
	h.Call("GET", base+"/s1/guests/gone", "", 404)
	if len(hosts.tracked) != 0 || len(h.Audits) != 0 {
		t.Fatalf("a read was recorded as a change: %v %v", hosts.tracked, h.Audits)
	}
}

func TestPowerIsRecordedAndSaysWhy(t *testing.T) {
	h, hosts := serve(t, false)
	// A change without a reason is refused before the server hears of it.
	h.Call("POST", base+"/s1/guests/web/stop", `{}`, 400)
	if len(hosts.hv.calls) != 0 {
		t.Fatalf("ran without a reason: %v", hosts.hv.calls)
	}
	for _, c := range []struct{ verb, call, status string }{
		{"stop", "stop web", "stopped"}, {"start", "start web", "running"},
		{"shutdown", "shutdown web", "stopped"}, {"restart", "reboot web", "running"},
	} {
		got := h.Call("POST", base+"/s1/guests/web/"+c.verb, `{"reason":"maintenance window"}`, 200)
		if got["status"] != c.status || hosts.hv.calls[len(hosts.hv.calls)-1] != c.call {
			t.Fatalf("%s: %v %v", c.verb, got, hosts.hv.calls)
		}
	}
	last := hosts.tracked[len(hosts.tracked)-1]
	if len(hosts.tracked) != 4 || last.ServerID != "s1" || last.Guest != "web" || last.Verb != "reboot" || last.Actor != "ada" || last.Note != "maintenance window" {
		t.Fatalf("operations: %+v", hosts.tracked)
	}
	if len(h.Audits) != 4 || len(h.Events) != 4 {
		t.Fatalf("audit %v events %v", h.Audits, h.Events)
	}
	h.Call("POST", base+"/s1/guests/gone/stop", `{"reason":"maintenance window"}`, 404)

	// With no record there is no change.
	hosts.noRecord = true
	before := len(hosts.hv.calls)
	h.Call("POST", base+"/s1/guests/web/stop", `{"reason":"maintenance window"}`, 502)
	if len(hosts.hv.calls) != before {
		t.Fatalf("ran unrecorded: %v", hosts.hv.calls)
	}
}

func TestThePanelsOwnGuestIsGuarded(t *testing.T) {
	stop := base + "/s1/guests/panel-vm/stop"
	h, hosts := serve(t, false)
	h.Call("POST", stop, `{"reason":"maintenance window"}`, 409)
	h.Call("POST", stop, `{"reason":"maintenance window","confirm_self":true}`, 403)
	h.Call("DELETE", base+"/s1/guests/panel-vm", `{"reason":"maintenance window","confirm":"panel-vm","confirm_self":true}`, 403)
	if len(hosts.hv.calls) != 0 {
		t.Fatalf("the panel's guest was touched: %v", hosts.hv.calls)
	}
	// Starting it is harmless.
	h.Call("POST", base+"/s1/guests/panel-vm/start", `{"reason":"maintenance window"}`, 200)

	h, hosts = serve(t, true)
	h.Call("POST", stop, `{"reason":"maintenance window"}`, 409)
	h.Call("POST", stop, `{"reason":"maintenance window","confirm_self":true}`, 200)
	if len(hosts.hv.calls) != 1 || hosts.hv.calls[0] != "stop panel-vm" {
		t.Fatalf("%v", hosts.hv.calls)
	}
}

func TestCreateAndDelete(t *testing.T) {
	h, hosts := serve(t, false)
	h.Call("POST", base+"/s1/guests", `{"reason":"new site","kind":"box","name":"api"}`, 400)
	h.Call("POST", base+"/s1/guests", `{"reason":"new site","kind":"ct","name":" "}`, 400)
	// This kind of server takes no storage for a new guest.
	h.Call("POST", base+"/s1/guests", `{"reason":"new site","kind":"ct","name":"api","storage":"local"}`, 400)
	if len(hosts.hv.calls) != 0 {
		t.Fatalf("an invalid guest reached the server: %v", hosts.hv.calls)
	}
	got := h.Call("POST", base+"/s1/guests", `{"reason":"new site","kind":"ct","name":"api","image":"images:debian/13","memory":512,"cores":1,"start":true}`, 201)
	made := hosts.hv.created
	if got["id"] != "api" || made.Image != "debian/13" || made.ImageServer != "https://images.example" || made.ImageProtocol != "simplestreams" || made.Memory != 512 || !made.Start {
		t.Fatalf("created %v from %+v", got, made)
	}
	if op := hosts.tracked[0]; op.Verb != "create" || op.Guest != "api" || op.Note != "new site" {
		t.Fatalf("operation: %+v", op)
	}

	// The guest's id is repeated, so a stale page cannot remove a different one.
	h.Call("DELETE", base+"/s1/guests/api", `{"reason":"retired"}`, 400)
	h.Call("DELETE", base+"/s1/guests/api", `{"reason":"retired","confirm":"web"}`, 400)
	h.Call("DELETE", base+"/s1/guests/api", `{"reason":"retired","confirm":"api"}`, 204)
	if _, still := hosts.hv.guests["api"]; still || hosts.hv.calls[len(hosts.hv.calls)-1] != "delete api" {
		t.Fatalf("not deleted: %v", hosts.hv.calls)
	}
	h.Call("DELETE", base+"/s1/guests/api", `{"reason":"retired","confirm":"api"}`, 404)
}

// An installation that sends power changes to an outside reviewer runs none without a review, and
// hands the reviewed path exactly what was asked.
func TestPowerGoesThroughTheOutsideReviewer(t *testing.T) {
	h, hosts := serve(t, false)
	hosts.review = true
	h.Call("POST", base+"/s1/guests/web/stop", `{"reason":"maintenance window"}`, 409)
	h.Call("POST", base+"/s1/guests/web/stop", `{"reason":"maintenance window","review":"r-1"}`, 200)
	if len(hosts.hv.calls) != 0 || len(hosts.tracked) != 0 {
		t.Fatalf("ran outside the review: %v", hosts.hv.calls)
	}
	want := ports.ReviewedPower{Server: "s1", Guest: "web", Verb: "stop", Review: "r-1", Actor: "ada", Reason: "maintenance window"}
	if len(hosts.reviewed) != 1 || hosts.reviewed[0] != want {
		t.Fatalf("reviewed: %+v", hosts.reviewed)
	}
}
