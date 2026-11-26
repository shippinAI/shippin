package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/mcpl"
)

// MCP (Model Context Protocol, streamable HTTP in JSON mode). Its tools are the generated actions the
// caller may run, nothing written by hand. A tool call is dispatched as an ordinary request to the
// action's own route with the caller's own credentials, so it passes the same gate, takes the same
// reason, commits with the same audit entry and emits the same event as the panel's call.
//
// POST /v1/mcp is the entrance. It holds no action tools of its own: it lists the boundaries the
// caller may work in, hands over one boundary's knowledge and tools, and runs a tool inside it.
// POST /v1/mcp/{domain} is one boundary: that domain's knowledge and its tools, directly.

var mcpVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26"}

const mcpEntrance = "This is the entrance to a Shippin installation, which runs a cloud: organizations, their workspaces, servers and services. " +
	"Work is divided into boundaries. Call `boundaries` to see the ones open to the person you act for, " +
	"`enter` to get one boundary's knowledge and its tools, then `run` to use a tool in it. " +
	"Enter a boundary before acting in it."

const mcpInstructions = "Every tool is an action the person you act for could take in the panel, with their permissions. " +
	"Changes are audited under their name: pass `reason` saying why, in their words. " +
	"Read-only tools are safe to call; ask the person before a destructive one."

type rpcReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (a *API) mcpRoutes(mux Router) {
	a.transport(mux, "POST /v1/mcp", Protocol, a.flow.Require(a.mcp))
	a.transport(mux, "POST /v1/mcp/{domain}", Protocol, a.flow.Require(a.mcp))
}

func (a *API) mcp(w http.ResponseWriter, r *http.Request) {
	domain := r.PathValue("domain")
	if domain != "" && !slices.Contains(mcpl.Domains, domain) {
		auth.Problem(w, http.StatusNotFound, "There is no MCP for "+domain+".")
		return
	}
	var req rpcReq
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil || req.JSONRPC != "2.0" || req.Method == "" {
		writeRPC(w, nil, nil, &rpcErr{-32700, "Send one JSON-RPC 2.0 request."})
		return
	}
	if len(req.ID) == 0 { // a notification
		w.WriteHeader(http.StatusAccepted)
		return
	}
	c, ok := a.resolve(w, r)
	if !ok {
		return
	}
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		version := mcpVersions[0]
		if slices.Contains(mcpVersions, p.ProtocolVersion) {
			version = p.ProtocolVersion
		}
		name := "shippin"
		if domain != "" {
			name += "-" + domain
		}
		instructions := mcpEntrance
		if domain != "" {
			instructions = mcpl.Boundaries[domain].Knowledge + " " + mcpInstructions
		}
		writeRPC(w, req.ID, map[string]any{"protocolVersion": version, "capabilities": map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo": map[string]any{"name": name, "title": "Shippin", "version": "mcpl-" + mcpl.Version}, "instructions": instructions}, nil)
	case "ping":
		writeRPC(w, req.ID, map[string]any{}, nil)
	case "tools/list":
		if domain == "" {
			writeRPC(w, req.ID, map[string]any{"tools": entranceTools}, nil)
			return
		}
		writeRPC(w, req.ID, map[string]any{"tools": a.boundaryTools(r, c, domain)}, nil)
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			writeRPC(w, req.ID, nil, &rpcErr{-32602, "tools/call takes a name and arguments."})
			return
		}
		if domain == "" {
			result, e := a.entranceCall(r, c, p.Name, p.Arguments)
			writeRPC(w, req.ID, result, e)
			return
		}
		result, e := a.boundaryCall(r, c, domain, p.Name, p.Arguments)
		writeRPC(w, req.ID, result, e)
	default:
		writeRPC(w, req.ID, nil, &rpcErr{-32601, req.Method + " isn't supported."})
	}
}

// entranceTools are the entrance's only tools. None of them is an action; each leads to a boundary.
var entranceTools = []map[string]any{
	entranceTool("boundaries", "List the boundaries open to the person you act for, each with what it is for and how many tools it holds.",
		mcpl.Schema{"type": "object", "properties": mcpl.Schema{}}, true),
	entranceTool("enter", "Get one boundary's knowledge and its tools, with their inputs. Do this before acting in it.",
		mcpl.Schema{"type": "object", "required": []string{"boundary"}, "properties": mcpl.Schema{
			"boundary": mcpl.Schema{"type": "string", "enum": mcpl.Domains, "description": "A name from `boundaries`."}}}, true),
	entranceTool("run", "Run one tool inside a boundary you have entered. It acts as the person, with their permissions, and is audited under their name.",
		mcpl.Schema{"type": "object", "required": []string{"boundary", "tool"}, "properties": mcpl.Schema{
			"boundary":  mcpl.Schema{"type": "string", "enum": mcpl.Domains, "description": "The boundary the tool is in."},
			"tool":      mcpl.Schema{"type": "string", "description": "The tool's name, as `enter` listed it."},
			"arguments": mcpl.Schema{"type": "object", "description": "The tool's input, as its schema from `enter` describes."}}}, false),
}

func entranceTool(name, desc string, schema mcpl.Schema, readOnly bool) map[string]any {
	return map[string]any{"name": name, "description": desc, "inputSchema": schema,
		"annotations": map[string]any{"readOnlyHint": readOnly, "openWorldHint": false}}
}

// boundaryTools are the actions in one domain that the caller may run.
func (a *API) boundaryTools(r *http.Request, c *caller, domain string) []map[string]any {
	tools := []map[string]any{}
	for _, g := range a.actionsFor(r, c) {
		if g.Domain() == domain {
			tools = append(tools, a.mcpTool(c, g))
		}
	}
	return tools
}

func (a *API) boundaryCall(r *http.Request, c *caller, domain, name string, args map[string]any) (map[string]any, *rpcErr) {
	for _, g := range a.actionsFor(r, c) {
		if g.Domain() == domain && toolName(g.ID) == name {
			return a.mcpCall(r, g, args), nil
		}
	}
	return nil, &rpcErr{-32602, "There is no tool " + name + " you may run."}
}

func (a *API) entranceCall(r *http.Request, c *caller, name string, args map[string]any) (map[string]any, *rpcErr) {
	boundary, _ := args["boundary"].(string)
	switch name {
	case "boundaries":
		list := []map[string]any{}
		for _, d := range mcpl.Domains {
			if n := len(a.boundaryTools(r, c, d)); n > 0 {
				list = append(list, map[string]any{"name": d, "purpose": mcpl.Boundaries[d].Purpose, "tools": n, "path": "/v1/mcp/" + d})
			}
		}
		return entranceResult(map[string]any{"boundaries": list}), nil
	case "enter":
		tools := a.boundaryTools(r, c, boundary)
		if len(tools) == 0 {
			return nil, &rpcErr{-32602, "There is no boundary " + boundary + " open to the person you act for. Call boundaries for the list."}
		}
		b := mcpl.Boundaries[boundary]
		return entranceResult(map[string]any{"boundary": boundary, "purpose": b.Purpose, "knowledge": b.Knowledge + " " + mcpInstructions, "tools": tools}), nil
	case "run":
		tool, _ := args["tool"].(string)
		inner, _ := args["arguments"].(map[string]any)
		return a.boundaryCall(r, c, boundary, tool, inner)
	}
	return nil, &rpcErr{-32602, "There is no tool " + name + " at the entrance. Its tools are boundaries, enter and run."}
}

func entranceResult(obj map[string]any) map[string]any {
	text, _ := json.Marshal(obj)
	return map[string]any{"isError": false, "structuredContent": obj, "content": []map[string]any{{"type": "text", "text": string(text)}}}
}

func writeRPC(w http.ResponseWriter, id json.RawMessage, result any, e *rpcErr) {
	out := map[string]any{"jsonrpc": "2.0", "id": id}
	if id == nil {
		out["id"] = nil
	}
	if e != nil {
		out["error"] = e
	} else {
		out["result"] = result
	}
	writeJSON(w, out)
}

// toolName is an action ID as a tool name model APIs accept: shippin.identity.ssh_key.create is
// identity-ssh_key-create.
func toolName(id string) string {
	return strings.ReplaceAll(strings.TrimPrefix(id, "shippin."), ".", "-")
}

func (a *API) mcpTool(c *caller, g mcpl.Registered) map[string]any {
	schema := mcpl.InputSchema(g)
	if g.Access == mcpl.Member {
		// The organizations this person may act in, so the model picks one rather than guessing.
		var ids, names []string
		for i := range c.tenants {
			if t := &c.tenants[i]; memberMay(c, t, g.Perm) {
				ids, names = append(ids, t.ID), append(names, t.ID+" is "+t.Name)
			}
		}
		schema["properties"].(mcpl.Schema)["tenantId"] = mcpl.Schema{"type": "string", "enum": ids,
			"description": "The organization to act in: " + strings.Join(names, "; ") + "."}
	}
	desc := g.Summary
	switch g.Risk {
	case mcpl.Destructive:
		desc += " Destructive: confirm with the person first."
	case mcpl.Write:
		desc += " Changes something, audited under the person's name."
	}
	method, _, _ := strings.Cut(g.Route, " ")
	return map[string]any{"name": toolName(g.ID), "title": g.ID, "description": desc, "inputSchema": schema,
		"annotations": map[string]any{"title": g.Summary, "readOnlyHint": g.Risk == mcpl.Read, "destructiveHint": g.Risk == mcpl.Destructive,
			"idempotentHint": method != http.MethodPost, "openWorldHint": false}}
}

// mcpCall runs g as an ordinary request to its own route, carrying the caller's credentials.
func (a *API) mcpCall(r *http.Request, g mcpl.Registered, args map[string]any) map[string]any {
	method, path, _ := strings.Cut(g.Route, " ")
	body := map[string]any{}
	for k, v := range args {
		body[k] = v
	}
	for _, p := range mcpl.RouteParams(g.Route) {
		v, _ := body[p].(string)
		if v == "" {
			return mcpResult(http.StatusBadRequest, []byte(`{"message":"`+p+` is required."}`))
		}
		path = strings.Replace(path, "{"+p+"}", url.PathEscape(v), 1)
		delete(body, p)
	}
	q := url.Values{}
	_, query := mcpl.Params(g.Runner.Input())
	for _, name := range query {
		if v, ok := body[name]; ok {
			q.Set(name, fmt.Sprint(v))
			delete(body, name)
		}
	}
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var reader io.Reader = http.NoBody
	if method != http.MethodGet {
		raw, _ := json.Marshal(body)
		reader = bytes.NewReader(raw)
	}
	inner := httptest.NewRequest(method, path, reader).WithContext(r.Context())
	inner.Host, inner.RemoteAddr = r.Host, r.RemoteAddr
	for _, h := range []string{"Authorization", "Cookie", "X-Forwarded-For", "X-Forwarded-Proto", "User-Agent"} {
		if v := r.Header.Get(h); v != "" {
			inner.Header.Set(h, v)
		}
	}
	inner.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	if a.root == nil {
		return mcpResult(http.StatusServiceUnavailable, []byte(`{"message":"Tools aren't wired in this process."}`))
	}
	a.root.ServeHTTP(rec, inner)
	return mcpResult(rec.Code, rec.Body.Bytes())
}

// mcpResult is an action's answer as a tool result: its JSON, structured when it is an object, and
// marked as an error with the plain message when the action refused.
func mcpResult(status int, body []byte) map[string]any {
	out := map[string]any{"isError": status >= 400}
	var obj map[string]any
	if json.Unmarshal(body, &obj) == nil && status < 400 {
		out["structuredContent"] = obj
	}
	text := strings.TrimSpace(string(body))
	switch {
	case status == http.StatusNoContent:
		text = "Done."
	case status >= 400:
		msg, _ := obj["message"].(string)
		if msg == "" {
			msg, _ = obj["title"].(string)
		}
		text = fmt.Sprintf("Refused (%d): %s", status, msg)
	}
	out["content"] = []map[string]any{{"type": "text", "text": text}}
	return out
}
