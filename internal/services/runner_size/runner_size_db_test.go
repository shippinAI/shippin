package runner_size_test

import (
	"strings"
	"testing"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/mcpl/mcpltest"
)

const base = "/v1/operator/services/runner-sizes"

func TestRunnerSizes(t *testing.T) {
	h := mcpltest.Serve(t, actions.All, "shippin.services.runner_size.", mcpltest.As{Actor: "ada"})
	if items := h.Call("GET", base, "", 200)["items"].([]any); len(items) != 0 {
		t.Fatalf("a fresh install offers no sizes, got %v", items)
	}
	const good = `{"id":"2x","label":"2 cores, 8 GB","cores":2,"memory_mib":8192,"disk_gib":40,"image":"runner-ubuntu-24.04","reason":"Offer a small runner."}`
	s := h.Call("POST", base, good, 201)
	if s["id"] != "2x" || s["active"] != true || s["cores"] != float64(2) {
		t.Fatalf("created: %v", s)
	}
	h.Call("POST", base, good, 409)
	for _, bad := range []string{
		`{"id":"2X","label":"x","cores":2,"memory_mib":8192,"disk_gib":40,"reason":"Bad id case."}`,
		`{"id":"big","label":"","cores":2,"memory_mib":8192,"disk_gib":40,"reason":"Missing label."}`,
		`{"id":"big","label":"Big","cores":0,"memory_mib":8192,"disk_gib":40,"reason":"No cores."}`,
		`{"id":"big","label":"Big","cores":2,"memory_mib":256,"disk_gib":40,"reason":"Too little memory."}`,
		`{"id":"big","label":"Big","cores":2,"memory_mib":8192,"disk_gib":4,"reason":"Too little disk."}`,
		`{"id":"big","label":"Big","cores":2,"memory_mib":8192,"disk_gib":40,"surprise":1,"reason":"Unknown field."}`,
	} {
		h.Call("POST", base, bad, 400)
	}
	h.Call("POST", base, `{"id":"big","label":"Big","cores":8,"memory_mib":32768,"disk_gib":100}`, 400) // a staff change needs a reason

	off := h.Call("PUT", base+"/2x", `{"label":"2 cores, 8 GB","cores":2,"memory_mib":8192,"disk_gib":40,"image":"runner-ubuntu-24.04","active":false,"reason":"Retire it."}`, 200)
	if off["active"] != false {
		t.Fatalf("turned off: %v", off)
	}
	h.Call("PUT", base+"/nope", `{"label":"x","cores":2,"memory_mib":8192,"disk_gib":40,"reason":"Nothing there."}`, 404)
	h.Call("GET", base+"/2x", "", 200)
	h.Call("GET", base+"/nope", "", 404)

	// A size a job is waiting for stays until the job ends.
	if _, err := h.Pool.Exec(t.Context(), `INSERT INTO tenants(id,idp_org_id,name,plan,status) VALUES('01a14149-0000-7000-8000-00000000000a','unlinked:a','Acme','pro','active');
	  INSERT INTO shippin_runner_jobs(id,github_job_id,tenant_id,repository_id,repository,size,status)
	  VALUES(gen_random_uuid(),1,'01a14149-0000-7000-8000-00000000000a',7,'acme/app','2x','queued')`); err != nil {
		t.Fatal(err)
	}
	h.Call("DELETE", base+"/2x", `{"reason":"Remove it."}`, 409)
	if _, err := h.Pool.Exec(t.Context(), `UPDATE shippin_runner_jobs SET status='completed'`); err != nil {
		t.Fatal(err)
	}
	h.Call("DELETE", base+"/2x", `{"reason":"Remove it."}`, 204)
	h.Call("DELETE", base+"/2x", `{"reason":"Remove it."}`, 404)

	var ids []string
	for _, e := range h.Events {
		ids = append(ids, e.ID)
	}
	if got := strings.Join(ids, " "); got != "shippin.services.runner_size.created shippin.services.runner_size.updated shippin.services.runner_size.deleted" {
		t.Fatalf("events: %s", got)
	}
	if len(h.Audits) != 3 || !strings.Contains(h.Audits[0], "Offer a small runner.") {
		t.Fatalf("audits: %q", h.Audits)
	}
}
