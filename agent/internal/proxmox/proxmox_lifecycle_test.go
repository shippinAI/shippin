package proxmox

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLifecycleObservesTaskAndGuest(t *testing.T) {
	state := "stopped"
	status := `{"data":{"status":"running"}}`
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api2/json/version":
			fmt.Fprint(w, `{"data":{"version":"8.4"}}`)
		case r.URL.Path == "/api2/json/cluster/resources":
			fmt.Fprintf(w, `{"data":[{"vmid":101,"node":"node-b","type":"qemu","status":%q}]}`, state)
		case r.URL.Path == "/api2/json/nodes/node-b/qemu/101/status/start" && r.Method == http.MethodPost:
			fmt.Fprint(w, `{"data":"UPID:node-b:abc:123:"}`)
		case strings.HasPrefix(r.URL.Path, "/api2/json/nodes/node-b/tasks/"):
			fmt.Fprint(w, status)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", 404)
		}
	}))
	defer server.Close()
	client := trustedClient(t, server)
	task, err := client.BeginAction(context.Background(), 101, "start")
	if err != nil || task.Node != "node-b" || task.UPID != "UPID:node-b:abc:123:" {
		t.Fatalf("task %#v, err %v", task, err)
	}
	if _, done, err := client.ObserveAction(context.Background(), task); err != nil || done {
		t.Fatalf("pending: done=%v err=%v", done, err)
	}
	status = `{"data":{"status":"stopped","exitstatus":"OK"}}`
	if _, done, err := client.ObserveAction(context.Background(), task); err == nil || done {
		t.Fatalf("unconfirmed state must fail: done=%v err=%v", done, err)
	}
	state = "running"
	if got, done, err := client.ObserveAction(context.Background(), task); err != nil || !done || got.ID != 101 {
		t.Fatalf("completed: guest=%#v done=%v err=%v", got, done, err)
	}
}

func TestLifecycleRejectsIncompleteAndFailedTasks(t *testing.T) {
	for _, body := range []string{`{"data":{"status":"stopped"}}`, `{"data":{"status":"stopped","exitstatus":"ERROR"}}`, `{"data":{"status":"unknown"}}`, `{"data":null}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			task := TaskRef{Node: "node-a", UPID: "UPID:node-a:abc:", VMID: 101, Action: "shutdown"}
			if _, done, err := trustedClient(t, server).ObserveAction(context.Background(), task); err == nil || done {
				t.Fatalf("unexpected success: done=%v err=%v", done, err)
			}
		})
	}
}

func TestLifecycleUnknownResponseAndInvalidInputs(t *testing.T) {
	posts := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			fmt.Fprint(w, `{"data":{"version":"8"}}`)
		case "/api2/json/cluster/resources":
			fmt.Fprint(w, `{"data":[{"vmid":101,"node":"node-a","type":"lxc","status":"stopped"}]}`)
		default:
			posts++
			fmt.Fprint(w, `{"data":null}`)
		}
	}))
	defer server.Close()
	client := trustedClient(t, server)
	if _, err := client.BeginAction(context.Background(), 101, "delete"); err == nil || posts != 0 {
		t.Fatal("accepted unsupported action")
	}
	if _, err := client.BeginAction(context.Background(), 101, "start"); !errors.Is(err, ErrOutcomeUnknown) || posts != 1 {
		t.Fatalf("expected unknown outcome, got %v", err)
	}
	if _, _, err := client.ObserveAction(context.Background(), TaskRef{Node: "../node", UPID: "UPID:../node:x:", VMID: 101, Action: "start"}); err == nil {
		t.Fatal("accepted unsafe task")
	}
}

func TestLifecycleRejectedAndInterruptedRequests(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api2/json/version":
			fmt.Fprint(w, `{"data":{"version":"8"}}`)
		case "/api2/json/cluster/resources":
			fmt.Fprint(w, `{"data":[{"vmid":101,"node":"node-a","type":"lxc","status":"running"}]}`)
		default:
			http.Error(w, "sensitive response", http.StatusForbidden)
		}
	}))
	defer server.Close()
	client := trustedClient(t, server)
	if _, err := client.BeginAction(context.Background(), 101, "shutdown"); err == nil || errors.Is(err, ErrOutcomeUnknown) || strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("expected sanitized rejection, got %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.BeginAction(ctx, 101, "start"); err == nil {
		t.Fatal("accepted cancelled request")
	}
}
