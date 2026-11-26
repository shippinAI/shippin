package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/mcpl"
)

func rpc(t *testing.T, mux http.Handler, path, body string) map[string]any {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	out := map[string]any{}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("%s: %d %s", body, rec.Code, rec.Body.String())
	}
	return out
}

// MCP's tools are the generated actions the caller may run, and a tool call goes through the action's
// own route and gate, never around it.
func TestMCPToolsAreTheCallersActions(t *testing.T) {
	for _, tc := range []struct {
		op        string
		staffTool bool
	}{{"op_superadmin", true}, {"", false}} {
		a := gateAPI(tc.op)
		a.flow = &auth.Flow{}
		mux := http.NewServeMux()
		a.Register(mux)

		init := rpc(t, mux, "/v1/mcp", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
		if init["result"].(map[string]any)["protocolVersion"] != "2025-06-18" {
			t.Fatalf("initialize: %v", init)
		}
		// The entrance holds no action of its own: only the way to a boundary.
		entrance := rpc(t, mux, "/v1/mcp", `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)["result"].(map[string]any)["tools"].([]any)
		var doors []string
		for _, x := range entrance {
			doors = append(doors, x.(map[string]any)["name"].(string))
		}
		if strings.Join(doors, " ") != "boundaries enter run" {
			t.Fatalf("the entrance lists %v", doors)
		}
		open := map[string]bool{}
		for _, x := range rpc(t, mux, "/v1/mcp", `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"boundaries","arguments":{}}}`)["result"].(map[string]any)["structuredContent"].(map[string]any)["boundaries"].([]any) {
			b := x.(map[string]any)
			if b["purpose"] == "" || b["tools"].(float64) < 1 {
				t.Errorf("boundary %v", b)
			}
			open[b["name"].(string)] = true
		}
		if !open["identity"] {
			t.Errorf("%q: identity is not open: %v", tc.op, open)
		}
		// Entering a boundary hands over its knowledge and exactly the caller's tools in it.
		names := map[string]map[string]any{}
		for d := range open {
			in := rpc(t, mux, "/v1/mcp", `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"enter","arguments":{"boundary":"`+d+`"}}}`)["result"].(map[string]any)["structuredContent"].(map[string]any)
			if !strings.HasPrefix(in["knowledge"].(string), mcpl.Boundaries[d].Knowledge) {
				t.Errorf("%s knowledge: %v", d, in["knowledge"])
			}
			for _, x := range in["tools"].([]any) {
				name := x.(map[string]any)["name"].(string)
				if !strings.HasPrefix(name, d+"-") {
					t.Errorf("%s holds %s", d, name)
				}
				names[name] = x.(map[string]any)
			}
		}
		if names["identity-ssh_key-create"] == nil {
			t.Errorf("%q: signed-in tools missing: %v", tc.op, names)
		}
		if got := names["tenancy-coverage_tier-update"] != nil; got != tc.staffTool {
			t.Errorf("%q: staff tool listed=%v, want %v", tc.op, got, tc.staffTool)
		}
		if upd := names["tenancy-coverage_tier-update"]; upd != nil {
			ann := upd["annotations"].(map[string]any)
			req := upd["inputSchema"].(map[string]any)["required"].([]any)
			if ann["readOnlyHint"] != false || len(req) != 2 {
				t.Errorf("update tool: %v %v", ann, req)
			}
		}
		domain := rpc(t, mux, "/v1/mcp/identity", `{"jsonrpc":"2.0","id":3,"method":"tools/list"}`)["result"].(map[string]any)["tools"].([]any)
		for _, x := range domain {
			if !strings.HasPrefix(x.(map[string]any)["name"].(string), "identity-") {
				t.Errorf("identity MCP lists %v", x)
			}
		}
		// Someone without the staff role is refused the tool, not served around the gate.
		call := rpc(t, mux, "/v1/mcp", `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"run","arguments":{"boundary":"tenancy","tool":"tenancy-coverage_tier-delete","arguments":{"coverageTierId":"x","reason":"testing the gate"}}}}`)
		direct := rpc(t, mux, "/v1/mcp/tenancy", `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"tenancy-coverage_tier-delete","arguments":{"coverageTierId":"x","reason":"testing the gate"}}}`)
		if (call["error"] == nil) != (direct["error"] == nil) {
			t.Errorf("%q: the entrance and the boundary disagree: %v %v", tc.op, call, direct)
		}
		// An action is not callable at the entrance itself.
		if rpc(t, mux, "/v1/mcp", `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"identity-ssh_key-create","arguments":{}}}`)["error"] == nil {
			t.Errorf("%q: an action ran at the entrance", tc.op)
		}
		if !tc.staffTool && call["error"] == nil {
			t.Errorf("%q: a tool the caller can't run was called: %v", tc.op, call)
		}
		if tc.staffTool {
			// It reached the action through the gate; this test has no database, so the action says so.
			res := call["result"].(map[string]any)
			if res["isError"] != true || !strings.Contains(res["content"].([]any)[0].(map[string]any)["text"].(string), "(503)") {
				t.Errorf("staff tool call: %v", call)
			}
		}
		if rpc(t, mux, "/v1/mcp", `{"jsonrpc":"2.0","id":5,"method":"nope"}`)["error"] == nil {
			t.Error("unknown method accepted")
		}
	}
}

func TestMCPResultAndNames(t *testing.T) {
	if toolName("shippin.identity.ssh_key.create") != "identity-ssh_key-create" {
		t.Error(toolName("shippin.identity.ssh_key.create"))
	}
	ok := mcpResult(200, []byte(`{"id":"a"}`))
	if ok["isError"] != false || ok["structuredContent"].(map[string]any)["id"] != "a" {
		t.Error(ok)
	}
	bad := mcpResult(403, []byte(`{"message":"Your role doesn't include that."}`))
	if bad["isError"] != true || !strings.Contains(bad["content"].([]map[string]any)[0]["text"].(string), "Your role") {
		t.Error(bad)
	}
}
