package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// serveMCP is a stdio MCP server (JSON-RPC 2.0, one message per line) exposing the signed-in
// account's devspaces to a local AI: list, exec, read/write files, resume. Every call goes
// through the same certificate and relay checks as a human shell, and is audited server-side.
func serveMCP(in io.Reader, out io.Writer) error {
	c, err := loadCreds()
	if err != nil {
		return err
	}
	t := &tools{c: c, conns: map[string]*sshConn{}}
	defer t.close()
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	enc := json.NewEncoder(out)
	for sc.Scan() {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if json.Unmarshal(sc.Bytes(), &req) != nil {
			continue
		}
		res, rerr := dispatch(t, req.Method, req.Params)
		if req.ID == nil { // notification
			continue
		}
		msg := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		if rerr != nil {
			msg["error"] = map[string]any{"code": -32601, "message": rerr.Error()}
		} else {
			msg["result"] = res
		}
		if err := enc.Encode(msg); err != nil {
			return err
		}
	}
	return sc.Err()
}

func dispatch(t *tools, method string, params json.RawMessage) (any, error) {
	switch method {
	case "initialize":
		return map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{"tools": map[string]any{}},
			"serverInfo": map[string]string{"name": "shippin-bridge", "version": "1"}}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": toolList}, nil
	case "tools/call":
		var p struct {
			Name string          `json:"name"`
			Args json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		text, err := t.call(p.Name, p.Args)
		if err != nil {
			return map[string]any{"isError": true, "content": []map[string]string{{"type": "text", "text": err.Error()}}}, nil
		}
		return map[string]any{"content": []map[string]string{{"type": "text", "text": text}}}, nil
	}
	return nil, fmt.Errorf("method not found: %s", method)
}

func schema(props map[string]string, req ...string) map[string]any {
	p := map[string]any{}
	for k, d := range props {
		p[k] = map[string]string{"type": "string", "description": d}
	}
	return map[string]any{"type": "object", "properties": p, "required": req}
}

var toolList = []map[string]any{
	{"name": "list_workspaces", "description": "Devspaces (and their projects) this Shippin account can open.", "inputSchema": schema(nil)},
	{"name": "exec", "description": "Run a shell command in a devspace; returns exit code, stdout, stderr.",
		"inputSchema": schema(map[string]string{"workspace": "id or name", "command": "shell command", "cwd": "optional directory"}, "workspace", "command")},
	{"name": "read_file", "description": "Read a text file (up to 1 MiB) from a devspace.",
		"inputSchema": schema(map[string]string{"workspace": "id or name", "path": "file path"}, "workspace", "path")},
	{"name": "write_file", "description": "Write a text file in a devspace (creates parent directories).",
		"inputSchema": schema(map[string]string{"workspace": "id or name", "path": "file path", "content": "full file content"}, "workspace", "path", "content")},
	{"name": "resume", "description": "Reconnect to a devspace and report where work left off: git repos under the home directory with branch, last commit and uncommitted changes, plus tmux sessions.",
		"inputSchema": schema(map[string]string{"workspace": "id or name"}, "workspace")},
}
