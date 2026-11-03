package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const clientUsage = `usage: rrflow session start --agent A [--mode M] | session end --id ID --status S | session list
       rrflow memory put KEY VALUE|- | memory get KEY | memory rm KEY | memory list
       rrflow decision add --action A --effect E [--resource R] [--reason T] [--session ID] | decision list`

// client talks to the bound rrflow/state instance. The token is read from RRFLOW_TOKEN_FILE and
// sent only to RRFLOW_URL; it is never printed.
func client(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, clientUsage)
		return 2
	}
	fs := flag.NewFlagSet("rrflow", flag.ContinueOnError)
	agent, mode := fs.String("agent", "", ""), fs.String("mode", "", "")
	id, status := fs.String("id", "", ""), fs.String("status", "", "")
	action, effect := fs.String("action", "", ""), fs.String("effect", "", "")
	resource, reason, session := fs.String("resource", "", ""), fs.String("reason", "", ""), fs.String("session", "", "")
	rest := args[2:]
	pos := []string{}
	for len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		pos, rest = append(pos, rest[0]), rest[1:]
	}
	if fs.Parse(rest) != nil {
		return 2
	}
	var method, path string
	var body any
	switch args[0] + " " + args[1] {
	case "session start":
		method, path, body = "POST", "/v1/sessions", map[string]string{"agent": *agent, "mode": *mode}
	case "session end":
		method, path, body = "POST", "/v1/sessions/"+url.PathEscape(*id)+"/end", map[string]string{"status": *status}
	case "session list":
		method, path = "GET", "/v1/sessions"
	case "memory list":
		method, path = "GET", "/v1/memory"
	case "memory get", "memory rm", "memory put":
		if len(pos) < 1 {
			fmt.Fprintln(os.Stderr, clientUsage)
			return 2
		}
		method, path = map[string]string{"get": "GET", "rm": "DELETE", "put": "PUT"}[args[1]], "/v1/memory/"+pos[0]
		if args[1] == "put" {
			v := strings.Join(pos[1:], " ")
			if v == "-" {
				b, _ := io.ReadAll(io.LimitReader(os.Stdin, 16<<10))
				v = string(b)
			}
			body = map[string]string{"value": v}
		}
	case "decision add":
		method, path = "POST", "/v1/decisions"
		body = map[string]string{"action": *action, "effect": *effect, "resource": *resource, "reason": *reason, "session": *session}
	case "decision list":
		method, path = "GET", "/v1/decisions"
	default:
		fmt.Fprintln(os.Stderr, clientUsage)
		return 2
	}
	out, code, err := call(method, path, body)
	if err != nil {
		fmt.Fprintln(os.Stderr, "rrflow:", err)
		return 1
	}
	if args[0]+" "+args[1] == "session start" && code == 201 {
		var s struct{ ID string }
		_ = json.Unmarshal(out, &s)
		fmt.Println(s.ID) // scripts capture the session id
		return 0
	}
	os.Stdout.Write(out)
	if code >= 300 {
		return 1
	}
	return 0
}

func call(method, path string, body any) ([]byte, int, error) {
	base, tf := os.Getenv("RRFLOW_URL"), os.Getenv("RRFLOW_TOKEN_FILE")
	if base == "" || tf == "" {
		return nil, 0, fmt.Errorf("rrflow is not attached to this devspace (RRFLOW_URL/RRFLOW_TOKEN_FILE unset)")
	}
	tok, err := os.ReadFile(tf)
	if err != nil {
		return nil, 0, fmt.Errorf("binding token unreadable")
	}
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, strings.TrimRight(base, "/")+path, rd)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(tok)))
	req.Header.Set("Content-Type", "application/json")
	res, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("rrflow unreachable")
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	return b, res.StatusCode, err
}
