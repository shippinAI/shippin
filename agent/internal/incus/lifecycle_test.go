package incus

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

const testOperationID = "11111111-2222-4333-8444-555555555555"

func lifecycleClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	client, err := NewLocal(localServer(t, handler))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func writeInventory(w http.ResponseWriter, r *http.Request, status string) bool {
	switch r.URL.Path {
	case "/1.0":
		fmt.Fprint(w, `{"type":"sync","status_code":200,"metadata":{"environment":{"server_version":"6.0"}}}`)
	case "/1.0/instances":
		fmt.Fprintf(w, `{"type":"sync","status_code":200,"metadata":[{"name":"vm-a","type":"container","status":%q}]}`, status)
	default:
		return false
	}
	return true
}

func TestLifecycleSubmitsThenConfirmsAgainstFreshState(t *testing.T) {
	state := "Stopped"
	operation := 103
	var submitted, query string
	client := lifecycleClient(t, func(w http.ResponseWriter, r *http.Request) {
		if writeInventory(w, r, state) {
			return
		}
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/1.0/instances/vm-a/state":
			body, _ := io.ReadAll(r.Body)
			submitted, query = string(body), r.URL.RawQuery
			w.WriteHeader(http.StatusAccepted)
			fmt.Fprintf(w, `{"type":"async","status_code":100,"metadata":{"id":%q}}`, testOperationID)
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/operations/"+testOperationID:
			fmt.Fprintf(w, `{"type":"sync","status_code":200,"metadata":{"id":%q,"status_code":%d}}`, testOperationID, operation)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	ref, err := client.BeginAction(context.Background(), "team & lab", "vm-a", "start")
	if err != nil {
		t.Fatal(err)
	}
	want := OperationRef{ID: testOperationID, Project: "team & lab", Instance: "vm-a", Action: "start"}
	if ref != want {
		t.Fatalf("got %#v, want %#v", ref, want)
	}
	if submitted != `{"action":"start","timeout":30}` || query != "project=team+%26+lab" {
		t.Fatalf("submitted %q with query %q", submitted, query)
	}
	if _, done, err := client.ObserveAction(context.Background(), ref); err != nil || done {
		t.Fatalf("pending: done=%v err=%v", done, err)
	}
	operation = 200
	if _, done, err := client.ObserveAction(context.Background(), ref); err == nil || done {
		t.Fatalf("unconfirmed state must fail: done=%v err=%v", done, err)
	}
	state = "Running"
	if got, done, err := client.ObserveAction(context.Background(), ref); err != nil || !done || got.Name != "vm-a" {
		t.Fatalf("completed: instance=%#v done=%v err=%v", got, done, err)
	}
}

func TestLifecycleRejectsFailedAndInvalidOperations(t *testing.T) {
	ref := OperationRef{ID: testOperationID, Project: "default", Instance: "vm-a", Action: "stop"}
	for _, tc := range []struct {
		name string
		body string
		code int
	}{
		{"failed", fmt.Sprintf(`{"type":"sync","status_code":200,"metadata":{"id":%q,"status_code":400}}`, ref.ID), http.StatusOK},
		{"cancelled", fmt.Sprintf(`{"type":"sync","status_code":200,"metadata":{"id":%q,"status_code":401}}`, ref.ID), http.StatusOK},
		{"other-operation", `{"type":"sync","status_code":200,"metadata":{"id":"99999999-2222-4333-8444-555555555555","status_code":200}}`, http.StatusOK},
		{"not-sync", fmt.Sprintf(`{"type":"async","status_code":200,"metadata":{"id":%q,"status_code":200}}`, ref.ID), http.StatusOK},
		{"malformed", `{`, http.StatusOK},
		{"missing", `sensitive response text`, http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := lifecycleClient(t, func(w http.ResponseWriter, r *http.Request) {
				if writeInventory(w, r, "Stopped") {
					return
				}
				w.WriteHeader(tc.code)
				fmt.Fprint(w, tc.body)
			})
			_, done, err := client.ObserveAction(context.Background(), ref)
			if err == nil || done {
				t.Fatalf("unexpected success: done=%v err=%v", done, err)
			}
			if strings.Contains(err.Error(), "sensitive") {
				t.Fatalf("provider text in error: %v", err)
			}
		})
	}
}

func TestLifecycleObserveFailsWhenInstanceDisappears(t *testing.T) {
	client := lifecycleClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/1.0":
			fmt.Fprint(w, `{"type":"sync","status_code":200,"metadata":{"environment":{"server_version":"6.0"}}}`)
		case "/1.0/instances":
			fmt.Fprint(w, `{"type":"sync","status_code":200,"metadata":[]}`)
		default:
			fmt.Fprintf(w, `{"type":"sync","status_code":200,"metadata":{"id":%q,"status_code":200}}`, testOperationID)
		}
	})
	ref := OperationRef{ID: testOperationID, Project: "default", Instance: "vm-a", Action: "stop"}
	if _, done, err := client.ObserveAction(context.Background(), ref); err == nil || done {
		t.Fatalf("unexpected success: done=%v err=%v", done, err)
	}
}

func TestLifecycleInvalidInputsNeverSubmit(t *testing.T) {
	puts := 0
	client := lifecycleClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			puts++
			return
		}
		writeInventory(w, r, "Stopped")
	})
	ctx := context.Background()
	for _, tc := range []struct{ project, name, action string }{
		{"default", "vm-a", "delete"},
		{"default", "../vm-a", "start"},
		{"default", "..", "start"},
		{"", "vm-a", "start"},
		{" default", "vm-a", "start"},
		{"default", "not-visible", "start"},
	} {
		if _, err := client.BeginAction(ctx, tc.project, tc.name, tc.action); err == nil {
			t.Errorf("accepted %+v", tc)
		}
	}
	if puts != 0 {
		t.Fatalf("submitted %d invalid actions", puts)
	}
	for _, ref := range []OperationRef{
		{ID: "../../1.0", Project: "default", Instance: "vm-a", Action: "start"},
		{ID: testOperationID, Project: "", Instance: "vm-a", Action: "start"},
		{ID: testOperationID, Project: "default", Instance: "a/b", Action: "start"},
		{ID: testOperationID, Project: "default", Instance: "vm-a", Action: "delete"},
	} {
		if _, _, err := client.ObserveAction(ctx, ref); err == nil {
			t.Errorf("accepted %+v", ref)
		}
	}
}

func TestLifecycleRejectedAndAmbiguousSubmissions(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		code int
	}{
		{"denied", `sensitive response text`, http.StatusForbidden},
		{"accepted-without-operation", `{"type":"async","status_code":100,"metadata":{}}`, http.StatusAccepted},
		{"accepted-with-unsafe-operation", `{"type":"async","status_code":100,"metadata":{"id":"../../../../1.0/instances"}}`, http.StatusAccepted},
		{"accepted-as-sync", fmt.Sprintf(`{"type":"sync","status_code":200,"metadata":{"id":%q}}`, testOperationID), http.StatusAccepted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := lifecycleClient(t, func(w http.ResponseWriter, r *http.Request) {
				if writeInventory(w, r, "Running") {
					return
				}
				w.WriteHeader(tc.code)
				fmt.Fprint(w, tc.body)
			})
			ref, err := client.BeginAction(context.Background(), "default", "vm-a", "stop")
			if err == nil || ref != (OperationRef{}) {
				t.Fatalf("got ref %#v, err %v", ref, err)
			}
			if strings.Contains(err.Error(), "sensitive") {
				t.Fatalf("provider text in error: %v", err)
			}
		})
	}
	client := lifecycleClient(t, func(w http.ResponseWriter, r *http.Request) { writeInventory(w, r, "Running") })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.BeginAction(ctx, "default", "vm-a", "stop"); err == nil {
		t.Fatal("accepted cancelled request")
	}
}
