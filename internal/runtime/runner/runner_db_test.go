package runner_test

import (
	"strings"
	"testing"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/mcpl/mcpltest"
	"shippin.cloud/console/internal/ports"
)

const base = "/v1/operator/runtime/runners"

// Staff see every organization's machines and remove a stuck one with a reason; the reaction deletes
// it from its host. The database refuses a machine of one organization for another's job.
func TestStaffRunners(t *testing.T) {
	s := mcpl.Services{}
	h := mcpltest.Serve(t, actions.All, "shippin.runtime.runner.", mcpltest.As{Actor: "ada", Services: s})
	h.React(actions.Reactions)
	w := newWorld(t, h.Pool)
	mcpl.Provide[ports.Machines](s, w.m)
	ctx := t.Context()

	if items := h.Call("GET", base, "", 200)["items"].([]any); len(items) != 0 {
		t.Fatalf("no machines yet: %v", items)
	}
	a1, g11 := w.queue(acme, 1), w.queue(globex, 11)
	w.provision(a1, false)
	w.provision(g11, false)
	ma, mg := w.machineFor(a1), w.machineFor(g11)

	items := h.Call("GET", base, "", 200)["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("staff see both organizations' machines: %v", items)
	}
	orgs := map[string]map[string]any{}
	for _, it := range items {
		r := it.(map[string]any)
		orgs[r["organization"].(string)] = r
	}
	acmeRow := orgs["Acme"]
	if acmeRow == nil || orgs["Globex"] == nil || acmeRow["id"] != ma.id || acmeRow["runner_job_id"] != a1 || acmeRow["github_job_id"] != "1" ||
		acmeRow["repository"] != "acme/app" || acmeRow["server"] != "runners-1" || acmeRow["state"] != "running" || acmeRow["guest"] != ma.guest {
		t.Fatalf("Acme's machine: %v", acmeRow)
	}
	for _, r := range orgs {
		if strings.Contains(strings.ToLower(jsonOf(r)), "secret") {
			t.Fatalf("a machine row carries the runner configuration: %v", r)
		}
	}
	h.Call("GET", base+"?state=bogus", "", 400)

	// Removing needs a reason, then deletes it from its host straight away.
	h.Call("DELETE", base+"/"+ma.id, "", 400)
	h.Call("DELETE", base+"/"+ma.id, `{"reason":"It hung on a network step."}`, 204)
	got := w.machineFor(a1)
	if got.state != "removed" || got.reason != "Removed by staff: It hung on a network step." || !contains(w.m.deleted, ma.guest) {
		t.Fatalf("after staff removed it: %+v, deleted %v", got, w.m.deleted)
	}
	if w.machineFor(g11).state != "running" {
		t.Fatal("removing Acme's machine touched Globex's")
	}
	h.Call("DELETE", base+"/"+ma.id, `{"reason":"Again."}`, 409)
	h.Call("DELETE", base+"/01a14149-0000-7000-8000-000000000999", `{"reason":"Nothing there."}`, 404)
	if removed := h.Call("GET", base+"?state=removed", "", 200)["items"].([]any); len(removed) != 1 {
		t.Fatalf("removed machines: %v", removed)
	}
	if len(h.Audits) != 1 || !strings.Contains(h.Audits[0], "shippin.runtime.runner.delete") || !strings.Contains(h.Audits[0], "It hung") {
		t.Fatalf("audits: %q", h.Audits)
	}
	if len(h.Events) != 1 || h.Events[0].ID != "shippin.runtime.runner.deleted" || h.Events[0].Subject["runnerId"] != ma.id {
		t.Fatalf("events: %+v", h.Events)
	}

	// The reference carries the organization: Globex's machine cannot point at Acme's job, nor be
	// recorded as having taken it.
	if _, err := h.Pool.Exec(ctx, `INSERT INTO shippin_runner_machines(id,tenant_id,runner_job_id,name,size,state)
	  VALUES(gen_random_uuid(),$1,$2::uuid,'shr-000000000001','2x','starting')`, globex, w.queue(acme, 2)); err == nil {
		t.Fatal("a Globex machine for an Acme job was stored")
	}
	if _, err := h.Pool.Exec(ctx, `UPDATE shippin_runner_machines SET assigned_job_id=$2::uuid WHERE id::text=$1`, mg.id, a1); err == nil {
		t.Fatal("a Globex machine was recorded as taking an Acme job")
	}
}

func jsonOf(m map[string]any) string {
	var b strings.Builder
	for k, v := range m {
		b.WriteString(k)
		b.WriteString("=")
		if s, ok := v.(string); ok {
			b.WriteString(s)
		}
		b.WriteString(" ")
	}
	return b.String()
}
