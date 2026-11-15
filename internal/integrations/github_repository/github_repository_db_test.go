package github_repository_test

import (
	"context"
	"errors"
	"testing"

	"shippin.cloud/console/internal/actions"
	"shippin.cloud/console/internal/mcpl"
	"shippin.cloud/console/internal/mcpl/mcpltest"
	"shippin.cloud/console/internal/ports"
)

// github is a fake GitHub App: what each installation reaches.
type github struct {
	reach map[int64][]ports.GitHubRepository
	down  bool
}

func (g *github) Repositories(_ context.Context, inst int64) ([]ports.GitHubRepository, error) {
	if g.down {
		return nil, errors.New("github is down")
	}
	return g.reach[inst], nil
}

const (
	acme   = "01a14149-0000-7000-8000-00000000000a"
	globex = "01a14149-0000-7000-8000-00000000000b"
	wsA    = "01a14149-0000-7000-8000-0000000000a1"
	wsA2   = "01a14149-0000-7000-8000-0000000000a2"
	prjA   = "01a14149-0000-7000-8000-0000000000a3"
	wsB    = "01a14149-0000-7000-8000-0000000000b1"
)

func TestRepositoryAssignment(t *testing.T) {
	g := &github{reach: map[int64][]ports.GitHubRepository{
		11: {{ID: 101, FullName: "acme/app", Private: true}, {ID: 102, FullName: "acme/site"}},
		22: {{ID: 201, FullName: "globex/api", Private: true}},
	}}
	s := mcpl.Services{}
	mcpl.Provide[ports.GitHub](s, g)
	h := mcpltest.Serve(t, actions.All, "shippin.integrations.github_repository.", mcpltest.As{Actor: "ada", Tenant: acme, Services: s})
	if _, err := h.Pool.Exec(t.Context(), `
	  INSERT INTO tenants(id,idp_org_id,name,plan,status) VALUES('`+acme+`','unlinked:a','Acme','pro','active'), ('`+globex+`','unlinked:b','Globex','pro','active');
	  INSERT INTO shippin_workspaces(id,tenant_id,name,created_by,is_default) VALUES('`+wsA+`','`+acme+`','Main','ada',false), ('`+wsA2+`','`+acme+`','Second','ada',false),
	    ('`+wsB+`','`+globex+`','Main','bob',false);
	  INSERT INTO projects(id,tenant_id,name,created_by,workspace_id) VALUES('`+prjA+`','`+acme+`','web','ada','`+wsA+`');
	  INSERT INTO github_installations(id,tenant_id,account_login,account_type) VALUES(11,'`+acme+`','acme','Organization'), (22,'`+globex+`','globex','Organization')`); err != nil {
		t.Fatal(err)
	}
	base := "/v1/tenants/" + acme + "/integrations/github-repositories"

	list := h.Call("GET", base, "", 200)["items"].([]any)
	if len(list) != 2 || list[0].(map[string]any)["workspace_id"] != nil {
		t.Fatalf("Acme sees its two unassigned repositories, not Globex's: %v", list)
	}
	r := h.Call("PUT", base+"/101", `{"workspace_id":"`+wsA+`","project_id":"`+prjA+`","runners":true}`, 200)
	if r["full_name"] != "acme/app" || r["project_id"] != prjA || r["runners"] != true || r["private"] != true {
		t.Fatalf("assigned: %v", r)
	}
	moved := h.Call("PUT", base+"/101", `{"workspace_id":"`+wsA2+`","runners":false}`, 200)
	if moved["workspace_id"] != wsA2 || moved["project_id"] != nil || moved["runners"] != false {
		t.Fatalf("moved: %v", moved)
	}

	// Refusals: another organization's repository, workspace or project, and nonsense.
	h.Call("PUT", base+"/201", `{"workspace_id":"`+wsA+`","runners":true}`, 404)           // Globex's repository
	h.Call("PUT", base+"/101", `{"workspace_id":"`+wsB+`","runners":true}`, 400)           // Globex's workspace
	h.Call("PUT", base+"/101", `{"workspace_id":"`+wsA2+`","project_id":"`+prjA+`"}`, 400) // a project of another workspace
	h.Call("PUT", base+"/abc", `{"workspace_id":"`+wsA+`"}`, 404)
	h.Call("PUT", base+"/101", `{"workspace_id":"`+wsA+`","owner":"x"}`, 400)
	g.down = true
	h.Call("PUT", base+"/102", `{"workspace_id":"`+wsA+`"}`, 409)
	h.Call("GET", base, "", 409)
	g.down = false

	// Globex, through its own route, sees only its own and cannot touch Acme's.
	h.As.Tenant = globex
	other := "/v1/tenants/" + globex + "/integrations/github-repositories"
	if l := h.Call("GET", other, "", 200)["items"].([]any); len(l) != 1 || l[0].(map[string]any)["id"] != "201" {
		t.Fatalf("Globex's list: %v", l)
	}
	h.Call("DELETE", other+"/101", "", 404)
	h.As.Tenant = acme

	// A repository GitHub stops listing stays visible while it is assigned.
	g.reach[11] = g.reach[11][1:]
	if l := h.Call("GET", base, "", 200)["items"].([]any); len(l) != 2 || l[1].(map[string]any)["id"] != "101" {
		t.Fatalf("a lost repository disappeared: %v", l)
	}
	h.Call("DELETE", base+"/101", "", 204)
	h.Call("DELETE", base+"/101", "", 404)

	// A deleted installation's repositories are no longer listed or assignable.
	if _, err := h.Pool.Exec(t.Context(), `UPDATE github_installations SET deleted_at=now() WHERE id=11`); err != nil {
		t.Fatal(err)
	}
	if l := h.Call("GET", base, "", 200)["items"].([]any); len(l) != 0 {
		t.Fatalf("a removed connection still lists: %v", l)
	}
	h.Call("PUT", base+"/102", `{"workspace_id":"`+wsA+`"}`, 404)

	if len(h.Events) != 3 || h.Events[0].ID != "shippin.integrations.github_repository.updated" || h.Events[0].Tenant != acme ||
		h.Events[2].ID != "shippin.integrations.github_repository.deleted" {
		t.Fatalf("events: %+v", h.Events)
	}

	// Without a GitHub App set up, the install says so plainly.
	plain := mcpltest.Serve(t, actions.All, "shippin.integrations.github_repository.", mcpltest.As{Actor: "ada", Tenant: acme})
	plain.Call("GET", base, "", 503)
}
