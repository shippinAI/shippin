package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"shippin.cloud/console/internal/runfleet"
)

// serveMCP speaks JSON-RPC 2.0 on stdio. Tool results are JSON text plus structuredContent.
func serveMCP(in io.Reader, out io.Writer, svc *runfleet.Service) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 4<<20)
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
		result, rpcErr := dispatch(context.Background(), svc, req.Method, req.Params)
		if len(req.ID) == 0 {
			continue
		}
		msg := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		if rpcErr != nil {
			msg["error"] = map[string]any{"code": rpcErr.code, "message": rpcErr.message}
		} else {
			msg["result"] = result
		}
		if err := enc.Encode(msg); err != nil {
			return err
		}
	}
	return sc.Err()
}

type rpcError struct {
	code    int
	message string
}

func dispatch(ctx context.Context, svc *runfleet.Service, method string, params json.RawMessage) (any, *rpcError) {
	switch method {
	case "initialize":
		return map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]string{"name": "shippin-run", "version": "1"},
		}, nil
	case "ping", "notifications/initialized":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": toolList}, nil
	case "tools/call":
		var call struct {
			Name string          `json:"name"`
			Args json.RawMessage `json:"arguments"`
		}
		if len(bytes.TrimSpace(params)) == 0 {
			return nil, &rpcError{-32602, "params are required"}
		}
		if err := json.Unmarshal(params, &call); err != nil {
			return nil, &rpcError{-32602, "invalid params"}
		}
		value, err := callTool(ctx, svc, call.Name, call.Args)
		if err != nil {
			return toolError(err), nil
		}
		return toolOK(value), nil
	default:
		return nil, &rpcError{-32601, "method not found"}
	}
}

func callTool(ctx context.Context, svc *runfleet.Service, name string, raw json.RawMessage) (any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = []byte(`{}`)
	}
	switch name {
	case "connect_repo":
		var in runfleet.ConnectInput
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return svc.Connect(ctx, in)
	case "list_runners":
		return svc.ListRunners()
	case "get_runner_health":
		var in struct {
			RunnerID string `json:"runner_id"`
		}
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		if in.RunnerID == "" {
			return nil, fmt.Errorf("runner_id is required")
		}
		return svc.RunnerHealth(ctx, in.RunnerID)
	case "list_jobs":
		var in struct {
			Repo string `json:"repo"`
		}
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return svc.ListJobs(in.Repo)
	case "get_job_logs":
		var in struct {
			JobID string `json:"job_id"`
		}
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		if in.JobID == "" {
			return nil, fmt.Errorf("job_id is required")
		}
		return svc.JobLogs(in.JobID)
	case "explain_failure":
		var in struct {
			JobID string `json:"job_id"`
		}
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		if in.JobID == "" {
			return nil, fmt.Errorf("job_id is required")
		}
		return svc.Explain(in.JobID)
	case "set_runs_on":
		var in runfleet.RunsOnInput
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return svc.SetRunsOn(ctx, in)
	case "get_usage":
		var in struct {
			Account string `json:"account"`
			Period  string `json:"period"`
		}
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return svc.Usage(in.Account, in.Period)
	case "set_log_retention":
		var in struct {
			Repo string `json:"repo"`
			Days int    `json:"days"`
		}
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		if err := svc.SetRetention(in.Repo, in.Days); err != nil {
			return nil, err
		}
		return map[string]any{"repo": in.Repo, "days": in.Days}, nil
	case "approve_repo":
		var in runfleet.ApproveInput
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return svc.Approve(ctx, in)
	default:
		return nil, fmt.Errorf("unknown tool %q", name)
	}
}

func decode(raw json.RawMessage, dest any) error {
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = []byte(`{}`)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		return err
	}
	return nil
}

func toolOK(v any) map[string]any {
	text, _ := json.MarshalIndent(v, "", "  ")
	return map[string]any{
		"content":           []map[string]string{{"type": "text", "text": string(text)}},
		"structuredContent": v,
		"isError":           false,
	}
}

func toolError(err error) map[string]any {
	payload := map[string]string{"message": err.Error()}
	text, _ := json.MarshalIndent(payload, "", "  ")
	return map[string]any{
		"content":           []map[string]string{{"type": "text", "text": string(text)}},
		"structuredContent": payload,
		"isError":           true,
	}
}

func schema(props map[string]any, required ...string) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": props, "additionalProperties": false, "required": required}
}

func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func boolProp(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

var toolList = []map[string]any{
	{"name": "connect_repo", "description": "Plan runner onboarding for a GitHub user or organization. Personal accounts are registered per repository. Workflow edits stay a preview unless apply_workflow is true.",
		"inputSchema": schema(map[string]any{
			"account": strProp("GitHub account login"), "account_kind": strProp("user or organization"),
			"repos": map[string]any{"type": "array", "items": map[string]string{"type": "string"}, "description": "owner/name repositories"},
			"size":  strProp("size id, such as linux-x64-2"), "registration": strProp("repository or organization; personal accounts cannot use organization"),
			"facts":          map[string]any{"type": "array", "description": "repository facts used when the GitHub App is not configured"},
			"apply_workflow": boolProp("open a pull request for the workflow edit"), "log_retention_days": map[string]any{"type": "integer", "description": "0 discards logs"},
		}, "account", "account_kind", "repos", "size")},
	{"name": "list_runners", "description": "List runners recorded in ci_runners.json. This does not probe them.", "inputSchema": schema(map[string]any{})},
	{"name": "get_runner_health", "description": "Probe one runner: GitHub registration, substrate, and host load.", "inputSchema": schema(map[string]any{"runner_id": strProp("runner id or name")}, "runner_id")},
	{"name": "list_jobs", "description": "List stored jobs, newest first, without log bodies.", "inputSchema": schema(map[string]any{"repo": strProp("optional owner/name filter")})},
	{"name": "get_job_logs", "description": "Return one job log with secrets redacted. Expired logs are omitted.", "inputSchema": schema(map[string]any{"job_id": strProp("job id")}, "job_id")},
	{"name": "explain_failure", "description": "Explain a stored job from its redacted log. This does not call a model.", "inputSchema": schema(map[string]any{"job_id": strProp("job id")}, "job_id")},
	{"name": "set_runs_on", "description": "Preview a runs-on change to shippin-<size>. Dry-run unless apply is true, which opens a pull request.",
		"inputSchema": schema(map[string]any{
			"repo": strProp("owner/name"), "size": strProp("size id"), "apply": boolProp("open a pull request; defaults to false"),
			"workflows": map[string]any{"type": "array", "description": "workflow files used when the GitHub App is not configured"},
			"lovable":   boolProp("the repository was synced by Lovable and has no workflow yet"),
		}, "repo", "size")},
	{"name": "get_usage", "description": "Metered minutes and list-price amount for one account and UTC month. This does not collect payment.",
		"inputSchema": schema(map[string]any{"account": strProp("GitHub account login"), "period": strProp("YYYY-MM; defaults to the current month")}, "account")},
	{"name": "set_log_retention", "description": "Set how many days a repository keeps logs. Zero discards logs immediately.",
		"inputSchema": schema(map[string]any{"repo": strProp("owner/name"), "days": map[string]any{"type": "integer"}}, "repo", "days")},
	{"name": "approve_repo", "description": "Approve a pending repository and record a desired runner. Guest creation may still be unavailable.",
		"inputSchema": schema(map[string]any{"connection_id": strProp("connection id"), "repo": strProp("owner/name"), "reason": strProp("why this was approved")}, "connection_id", "repo", "reason")},
}
