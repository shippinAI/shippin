package incus

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"shippin.cloud/console/internal/hv"
)

func guestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	trusted := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	c, err := New(Endpoint{BaseURL: srv.URL, TrustedPEM: trusted}, func() (string, error) { return "token", nil })
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// What Incus 6.0.4 sends, captured from a real server. Both of these are HTTP 200.
const (
	waitRanOut = `{"type":"error","status":"","status_code":0,"operation":"","error_code":500,"error":"context deadline exceeded","metadata":null}`
	failedWait = `{"type":"error","status":"","status_code":0,"operation":"","error_code":500,"error":"Failed to run: incusd forkstart web-1: exit status 1","metadata":null}`
	opRunning  = `{"type":"sync","status":"Success","status_code":200,"metadata":{"status":"Running","status_code":103,"err":""}}`
	opFailed   = `{"type":"sync","status":"Success","status_code":200,"metadata":{"status":"Failure","status_code":400,"err":"Failed to run: incusd forkstart web-1: exit status 1"}}`
	opDone     = `{"type":"sync","status":"Success","status_code":200,"metadata":{"status":"Success","status_code":200,"err":""}}`
	opGone     = `{"type":"error","status":"","status_code":0,"operation":"","error_code":404,"error":"Operation not found","metadata":null}`
)

const started = `{"type":"async","metadata":{"id":"op1"}}`

// A long download outlasts the time Incus holds a request. That is not a failure.
func TestWaitAsksAgainWhenIncusLetsGo(t *testing.T) {
	waitAgain = time.Millisecond
	var mu sync.Mutex
	waits := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if !strings.HasSuffix(r.URL.Path, "/wait") {
			_, _ = w.Write([]byte(opRunning))
			return
		}
		waits++
		if waits < 3 {
			_, _ = w.Write([]byte(waitRanOut))
			return
		}
		_, _ = w.Write([]byte(opDone))
	}))
	defer srv.Close()
	c := guestClient(t, srv)
	if err := c.wait(context.Background(), []byte(started)); err != nil || waits != 3 {
		t.Fatalf("asked %d times: %v", waits, err)
	}
}

// It can finish in the moment between the wait running out and the read.
func TestWaitSeesWorkThatFinishedJustAsItLetGo(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/wait") {
			_, _ = w.Write([]byte(waitRanOut))
			return
		}
		_, _ = w.Write([]byte(opDone))
	}))
	defer srv.Close()
	if err := guestClient(t, srv).wait(context.Background(), []byte(started)); err != nil {
		t.Fatal(err)
	}
}

func TestAFailedOperationKeepsItsReason(t *testing.T) {
	for name, read := range map[string]struct {
		code int
		body string
	}{
		"still remembered":  {http.StatusOK, opFailed},
		"already forgotten": {http.StatusNotFound, opGone},
	} {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/wait") {
				_, _ = w.Write([]byte(failedWait))
				return
			}
			w.WriteHeader(read.code)
			_, _ = w.Write([]byte(read.body))
		}))
		err := guestClient(t, srv).wait(context.Background(), []byte(started))
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), "forkstart web-1: exit status 1") {
			t.Errorf("%s: the reason was lost: %v", name, err)
		}
	}
}

func TestWaitStopsWhenAskedTo(t *testing.T) {
	waitAgain = time.Millisecond
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/wait") {
			_, _ = w.Write([]byte(waitRanOut))
			return
		}
		_, _ = w.Write([]byte(opRunning))
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := guestClient(t, srv).wait(ctx, []byte(started)); err == nil {
		t.Fatal("it must stop when the context ends")
	}
}

func TestCreateGuestSendsWhatIncusExpects(t *testing.T) {
	var mu sync.Mutex
	var sent map[string]any
	started := false
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/1.0/instances":
			_ = json.NewDecoder(r.Body).Decode(&sent)
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"type":"async","metadata":{"id":"op1"}}`))
		case r.URL.Path == "/1.0/operations/op1/wait":
			_, _ = w.Write([]byte(`{"metadata":{"status_code":200}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/1.0/instances/web-1/state":
			started = true
			_, _ = w.Write([]byte(`{"type":"sync","metadata":{}}`))
		case r.URL.Path == "/1.0/instances/web-1":
			status := "Stopped"
			if started {
				status = "Running"
			}
			_, _ = w.Write([]byte(`{"metadata":{"name":"web-1","type":"container","status":"` + status + `"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := guestClient(t, srv)
	g, err := c.CreateGuest(context.Background(), hv.GuestSpec{Kind: hv.KindCT, Name: "web-1", Image: "images:debian/13", Memory: 512, Cores: 2, Start: true})
	if err != nil || !strings.EqualFold(g.Status, "running") {
		t.Fatalf("%+v %v", g, err)
	}
	src := sent["source"].(map[string]any)
	cfg := sent["config"].(map[string]any)
	if sent["type"] != "container" || src["alias"] != "debian/13" || src["protocol"] != "simplestreams" || src["server"] != publicImages || cfg["limits.memory"] != "512MiB" || cfg["limits.cpu"] != "2" {
		t.Fatalf("%+v", sent)
	}
	for name, spec := range map[string]hv.GuestSpec{
		"a bad name":    {Kind: hv.KindCT, Name: "1 bad", Image: "images:debian/13"},
		"no image":      {Kind: hv.KindCT, Name: "ok"},
		"another image": {Kind: hv.KindCT, Name: "ok", Image: "docker:nginx"},
	} {
		if _, err := c.CreateGuest(context.Background(), spec); err == nil {
			t.Errorf("%s must be refused before anything is sent", name)
		}
	}
}

func TestAGuestThatWillNotStartIsStillSaidToExist(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/1.0/instances":
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"type":"async","metadata":{"id":"made"}}`))
		case r.URL.Path == "/1.0/operations/made/wait":
			_, _ = w.Write([]byte(`{"metadata":{"status_code":200}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/1.0/instances/web-1/state":
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"type":"async","metadata":{"id":"start"}}`))
		case r.URL.Path == "/1.0/operations/start/wait":
			_, _ = w.Write([]byte(failedWait))
		case r.URL.Path == "/1.0/operations/start":
			_, _ = w.Write([]byte(opFailed))
		case r.URL.Path == "/1.0/instances/web-1":
			_, _ = w.Write([]byte(`{"metadata":{"name":"web-1","type":"container","status":"Stopped"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := guestClient(t, srv)
	_, err := c.CreateGuest(context.Background(), hv.GuestSpec{Kind: hv.KindCT, Name: "web-1", Image: "images:alpine/3.21", Start: true})
	if err == nil || !strings.Contains(err.Error(), "web-1 was created but did not start") || !strings.Contains(err.Error(), "exit status 1") {
		t.Fatalf("%v", err)
	}
}

func TestDeleteRequiresNotFoundReadback(t *testing.T) {
	for _, tc := range []struct {
		name       string
		statusCode int
		wantError  bool
	}{
		{name: "confirmed absent", statusCode: http.StatusNotFound},
		{name: "server cannot confirm", statusCode: http.StatusBadGateway, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deleted := false
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/1.0/instances/web-1":
					if deleted {
						w.WriteHeader(tc.statusCode)
						_, _ = w.Write([]byte("read-back unavailable"))
						return
					}
					_, _ = w.Write([]byte(`{"metadata":{"name":"web-1","type":"container","status":"Stopped"}}`))
				case r.Method == http.MethodDelete && r.URL.Path == "/1.0/instances/web-1":
					deleted = true
					w.WriteHeader(http.StatusAccepted)
					_, _ = w.Write([]byte(`{"type":"async","metadata":{"id":"delete-1"}}`))
				case r.Method == http.MethodGet && r.URL.Path == "/1.0/operations/delete-1/wait":
					_, _ = w.Write([]byte(opDone))
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			err := guestClient(t, srv).DeleteGuest(context.Background(), "web-1")
			if tc.wantError {
				if err == nil || errors.Is(err, hv.ErrGuestNotFound) || !strings.Contains(err.Error(), "couldn't confirm guest web-1 was deleted") {
					t.Fatalf("want unconfirmed deletion error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("explicit 404 confirms deletion: %v", err)
			}
		})
	}
}
