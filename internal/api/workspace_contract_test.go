package api

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"shippin.cloud/console/internal/ports"
	"shippin.cloud/console/internal/runtime/devspace"
	"testing"
	"time"
)

// A development space is answered in the shape schemas/workspaces.yaml describes, whatever its host
// reports. The lifecycle routes this test also covered are generated actions now; their acceptance is
// asserted in devspace_actions_db_test.go and described by the generated OpenAPI.
func TestWorkspaceViewContract(t *testing.T) {
	spec, py := specFile(), pyYAML(t)
	cmd := exec.Command(py, "-c", `import json, pathlib, sys, yaml
p=pathlib.Path(sys.argv[1]); names=['schemas/workspaces.yaml','schemas/jobs.yaml','schemas/common.yaml']
print(json.dumps({n:yaml.safe_load((p/n).read_text(encoding='utf-8')) for n in names}))`, filepath.Dir(spec))
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("local PyYAML contract reader: %v: %s", err, raw)
	}
	docs := map[string]map[string]any{}
	if err := json.Unmarshal(raw, &docs); err != nil {
		t.Fatal(err)
	}
	a := &API{cfg: testCfg()}
	id := "10000000-0000-4000-8000-000000000001"
	ws := devspace.Devspace{ID: id, TenantID: id, Name: "test", State: "running", CreatedAt: time.Now()}
	for _, live := range []map[string]ports.Guest{nil, {}, {"w-" + id: {Status: "Running"}}, {"w-" + id: {Status: "Frozen"}}} {
		b, _ := json.Marshal(devspace.Reader{Shape: a.cfg.Workspaces}.Show(devspace.Org{}, ws, live))
		var v any
		json.Unmarshal(b, &v)
		if err := contractValue(docs, "schemas/workspaces.yaml", docs["schemas/workspaces.yaml"]["Workspace"].(map[string]any), v); err != nil {
			t.Fatal(err)
		}
	}
}
