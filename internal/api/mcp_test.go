package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shippin.cloud/console/internal/auth"
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
		tools := rpc(t, mux, "/v1/mcp", `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)["result"].(map[string]any)["tools"].([]any)
		names := map[string]map[string]any{}
		for _, x := range tools {
			names[x.(map[string]any)["name"].(string)] = x.(map[string]any)
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
		call := rpc(t, mux, "/v1/mcp", `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"tenancy-coverage_tier-delete","arguments":{"coverageTierId":"x","reason":"testing the gate"}}}`)
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
