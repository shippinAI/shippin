package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hostclient"
	"shippin.cloud/console/internal/jobs"
	"strings"
	"testing"
	"time"
)

// This bounded contract check validates methods, supported actions, status codes and
// actual JSON required fields/types/enums. It is not a complete OpenAPI validator.
func TestWorkspaceLifecycleContract(t *testing.T) {
	spec, py := specFile(), pyYAML(t)
	cmd := exec.Command(py, "-c", `import json, pathlib, sys, yaml
p=pathlib.Path(sys.argv[1]); names=['paths/operator-runtime.yaml','paths/workspace-actions.yaml','schemas/workspaces.yaml','schemas/jobs.yaml','schemas/common.yaml']
print(json.dumps({n:yaml.safe_load((p/n).read_text(encoding='utf-8')) for n in names}))`, filepath.Dir(spec))
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("local PyYAML contract reader: %v: %s", err, raw)
	}
	docs := map[string]map[string]any{}
	if err := json.Unmarshal(raw, &docs); err != nil {
		t.Fatal(err)
	}
	action := docs["paths/workspace-actions.yaml"]["action"].(map[string]any)
	params := action["parameters"].([]any)
	enum := params[2].(map[string]any)["schema"].(map[string]any)["enum"].([]any)
	if len(enum) != len(actionState) {
		t.Fatal("action enum differs")
	}
	for _, v := range enum {
		if _, ok := actionState[v.(string)]; !ok {
			t.Fatal("unsupported action", v)
		}
	}
	rec := &recorder{patterns: servedPatterns(t)}
	methodFound := false
	for _, p := range rec.patterns {
		if strings.HasPrefix(p, "POST ") && norm(strings.TrimPrefix(p, "POST ")) == "/tenants/{}/workspaces/{}/actions/{}" {
			methodFound = true
		}
	}
	if !methodFound {
		t.Fatal("lifecycle POST method missing")
	}
	responses := action["post"].(map[string]any)["responses"].(map[string]any)
	for _, code := range []string{"202", "400", "401", "403", "404", "409", "422", "503"} {
		if responses[code] == nil {
			t.Fatal("missing response", code)
		}
	}
	accepted := responses["202"].(map[string]any)
	if accepted["headers"].(map[string]any)["Location"] == nil {
		t.Fatal("missing polling location")
	}
	acceptSchema := accepted["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	a := &API{cfg: testCfg()}
	id := "10000000-0000-4000-8000-000000000001"
	ws := db.Workspace{ID: id, TenantID: id, Name: "test", State: "running", CreatedAt: time.Now()}
	for _, live := range []map[string]hostclient.Instance{nil, {}, {"w-" + id: {Status: "Running"}}, {"w-" + id: {Status: "Frozen"}}} {
		b, _ := json.Marshal(a.view(&tenantView{}, ws, live))
		var v any
		json.Unmarshal(b, &v)
		if err := contractValue(docs, "schemas/workspaces.yaml", docs["schemas/workspaces.yaml"]["Workspace"].(map[string]any), v); err != nil {
			t.Fatal(err)
		}
	}
	// Exercise the actual acceptance handler, with an isolated DB and an idle runner.
	a, d, tn := notifyAPI(t)
	a.jobs = jobs.NewStore(d.Pool)
	a.runner = jobs.NewRunner(a.jobs, jobs.Settings{})
	row := provisioningWS(t, a, d, tn)
	if err := d.SetWorkspaceState(t.Context(), row.ID, "stopped", "", ""); err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	a.enqueueWorkspaceAction(rr, req, &tenantView{Tenant: tn}, *row, "start", "u")
	if rr.Code != 202 || !strings.HasPrefix(rr.Header().Get("Location"), "/v1/jobs/") {
		t.Fatalf("acceptance: %d %s", rr.Code, rr.Body.String())
	}
	var v any
	if err := json.Unmarshal(rr.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if err := contractValue(docs, "paths/workspace-actions.yaml", acceptSchema, v); err != nil {
		t.Fatal(err)
	}

	op := docs["paths/operator-runtime.yaml"]["operatorWorkspaceAction"].(map[string]any)
	opEnum := op["parameters"].([]any)[1].(map[string]any)["schema"].(map[string]any)["enum"].([]any)
	if len(opEnum) != 3 || opEnum[0] != "start" || opEnum[1] != "stop" || opEnum[2] != "delete" {
		t.Fatal("operator actions differ")
	}
	opPost := op["post"].(map[string]any)
	opSchema := opPost["responses"].(map[string]any)["202"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	if err := contractValue(docs, "paths/operator-runtime.yaml", opSchema, v); err != nil {
		t.Fatal(err)
	}
	// The operator handler must reject unsupported actions and missing/wrong delete
	// confirmation before enqueue. Exercise the correct confirmation through the
	// actual handler using the existing member-manager authorization path.
	member := &caller{id: &auth.Identity{Sub: "u"}, managers: []string{"owner"}, tenants: []tenantView{{Tenant: tn, Roles: []string{"owner"}}}}
	for _, tc := range []struct {
		action, body string
		want         int
	}{{"restart", "", 400}, {"delete", `{}`, 400}, {"delete", `{"confirm":"wrong"}`, 400}} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/operator/action", strings.NewReader(tc.body))
		req.SetPathValue("workspaceId", row.ID)
		req.SetPathValue("action", tc.action)
		a.opWorkspaceAction(rr, req, member)
		if rr.Code != tc.want {
			t.Fatalf("operator guard %s: %d", tc.action, rr.Code)
		}
	}
	row2 := provisioningWS(t, a, d, tn)
	if err := d.SetWorkspaceState(t.Context(), row2.ID, "stopped", "", ""); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"confirm": row2.Name})
	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/operator/action", strings.NewReader(string(body)))
	req2.SetPathValue("workspaceId", row2.ID)
	req2.SetPathValue("action", "delete")
	a.opWorkspaceAction(rr2, req2, member)
	if rr2.Code != 202 {
		t.Fatalf("operator accepted: %d %s", rr2.Code, rr2.Body.String())
	}
	var opValue any
	if err := json.Unmarshal(rr2.Body.Bytes(), &opValue); err != nil {
		t.Fatal(err)
	}
	if err := contractValue(docs, "paths/operator-runtime.yaml", opSchema, opValue); err != nil {
		t.Fatal(err)
	}
	delete(v.(map[string]any), "job")
	if contractValue(docs, "paths/workspace-actions.yaml", acceptSchema, v) == nil {
		t.Fatal("missing required job was accepted")
	}
}
