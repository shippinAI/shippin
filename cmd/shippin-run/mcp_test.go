package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"shippin.cloud/console/internal/runfleet"
)

func TestMCPConnectAndDryRun(t *testing.T) {
	svc := openTest(t)
	var in, out bytes.Buffer
	writeRPC(&in, 1, "initialize", nil)
	writeRPC(&in, 2, "tools/list", nil)
	writeRPC(&in, 3, "tools/call", map[string]any{
		"name": "connect_repo",
		"arguments": map[string]any{
			"account": "octocat", "account_kind": "user", "registration": "organization",
			"repos": []string{"octocat/hello"}, "size": "linux-x64-2",
			"facts": []map[string]any{{"full_name": "octocat/hello", "license": "MIT", "lovable": true}},
		},
	})
	writeRPC(&in, 4, "tools/call", map[string]any{
		"name": "set_runs_on",
		"arguments": map[string]any{
			"repo": "octocat/hello", "size": "linux-x64-2",
			"workflows": []map[string]string{{"path": ".github/workflows/ci.yml", "content": "runs-on: ubuntu-latest\n"}},
		},
	})
	writeRPC(&in, 5, "tools/call", map[string]any{"name": "nope", "arguments": map[string]any{}})
	if err := serveMCP(&in, &out, svc); err != nil {
		t.Fatal(err)
	}
	msgs := decodeLines(t, &out)
	if len(msgs) != 5 {
		t.Fatalf("messages %d", len(msgs))
	}
	tools := msgs[1]["result"].(map[string]any)["tools"].([]any)
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.(map[string]any)["name"].(string)] = true
	}
	for _, name := range []string{"connect_repo", "list_runners", "get_runner_health", "list_jobs", "get_job_logs", "explain_failure", "set_runs_on", "get_usage"} {
		if !names[name] {
			t.Fatalf("missing tool %s", name)
		}
	}
	connected := structured(t, msgs[2])
	if connected["registration"] != "repository" || connected["runs_on"] != "shippin-linux-x64-2" {
		t.Fatalf("%v", connected)
	}
	repos := connected["repos"].([]any)
	repo := repos[0].(map[string]any)
	if repo["approval"] != "auto" {
		t.Fatal(repo)
	}
	provision := repo["provision"].(map[string]any)
	if provision["implemented"] != false {
		t.Fatal(provision)
	}
	plan := structured(t, msgs[3])
	if plan["dry_run"] != true || plan["pull_url"] != nil {
		t.Fatalf("set_runs_on was not a dry-run: %v", plan)
	}
	failed := msgs[4]["result"].(map[string]any)
	if failed["isError"] != true {
		t.Fatal(failed)
	}
	text := failed["content"].([]any)[0].(map[string]any)["text"].(string)
	if !json.Valid([]byte(text)) {
		t.Fatal(text)
	}
}

func TestMCPLogsRedact(t *testing.T) {
	svc := openTest(t)
	ctx := context.Background()
	res, err := svc.Connect(ctx, runfleet.ConnectInput{
		Account: "octocat", AccountKind: "user", Repos: []string{"octocat/hello"}, Size: "linux-x64-2",
		Facts: []runfleet.RepoFact{{FullName: "octocat/hello", License: "MIT"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	secret := "ghp_" + strings.Repeat("c", 36)
	summary, err := svc.IngestJob(runfleet.IngestInput{
		Repo: "octocat/hello", Name: "build", Conclusion: "failure", Log: "token=" + secret,
		StartedAt: time.Date(2026, 11, 13, 1, 0, 0, 0, time.UTC), FinishedAt: time.Date(2026, 11, 13, 1, 5, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	var in, out bytes.Buffer
	writeRPC(&in, 1, "tools/call", map[string]any{"name": "get_job_logs", "arguments": map[string]string{"job_id": summary.ID}})
	writeRPC(&in, 2, "tools/call", map[string]any{"name": "explain_failure", "arguments": map[string]string{"job_id": summary.ID}})
	writeRPC(&in, 3, "tools/call", map[string]any{"name": "get_usage", "arguments": map[string]string{"account": "octocat", "period": "2026-11"}})
	writeRPC(&in, 4, "tools/call", map[string]any{"name": "list_runners", "arguments": map[string]any{}})
	writeRPC(&in, 5, "tools/call", map[string]any{"name": "get_runner_health", "arguments": map[string]string{"runner_id": res.Repos[0].Runner.ID}})
	if err := serveMCP(&in, &out, svc); err != nil {
		t.Fatal(err)
	}
	msgs := decodeLines(t, &out)
	logs := structured(t, msgs[0])
	if strings.Contains(logs["log"].(string), secret) {
		t.Fatalf("secret leaked: %v", logs["log"])
	}
	explained := structured(t, msgs[1])
	signals := explained["signals"].([]any)
	if len(signals) == 0 {
		t.Fatal(explained)
	}
	usage := structured(t, msgs[2])
	if usage["note"] == "" || usage["account"] != "octocat" {
		t.Fatal(usage)
	}
	health := structured(t, msgs[4])
	if health["overall"] == "" {
		t.Fatal(health)
	}
}

func openTest(t *testing.T) *runfleet.Service {
	t.Helper()
	svc, err := runfleet.Open(t.TempDir(), runfleet.Options{
		Now: func() time.Time { return time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC) }, FreeMinutes: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func writeRPC(buf *bytes.Buffer, id int, method string, params any) {
	msg := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	b, _ := json.Marshal(msg)
	buf.Write(b)
	buf.WriteByte('\n')
}

func decodeLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	sc := bufio.NewScanner(buf)
	for sc.Scan() {
		var msg map[string]any
		if err := json.Unmarshal(sc.Bytes(), &msg); err != nil {
			t.Fatal(err)
		}
		out = append(out, msg)
	}
	return out
}

func structured(t *testing.T, msg map[string]any) map[string]any {
	t.Helper()
	result := msg["result"].(map[string]any)
	if result["isError"] == true {
		t.Fatalf("tool error: %v", result)
	}
	text := result["content"].([]any)[0].(map[string]any)["text"].(string)
	if !json.Valid([]byte(text)) {
		t.Fatalf("text is not JSON: %s", text)
	}
	raw, err := json.Marshal(result["structuredContent"])
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	return body
}
