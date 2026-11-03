package main

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectCommandReadsSelectedProject(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "incus.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method: %s", r.Method)
		}
		switch r.URL.Path {
		case "/1.0":
			fmt.Fprint(w, `{"type":"sync","status_code":200,"metadata":{"environment":{"server_version":"6.0"}}}`)
		case "/1.0/instances":
			if r.URL.Query().Get("project") != "example" {
				t.Errorf("unexpected project: %q", r.URL.Query().Get("project"))
			}
			fmt.Fprint(w, `{"type":"sync","status_code":200,"metadata":[]}`)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), []string{"inspect-incus", "-socket", socket, "-project", "example"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Provider      string `json:"provider"`
		Project       string `json:"project"`
		ServerVersion string `json:"server_version"`
		Instances     []any  `json:"instances"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || result.Provider != "incus" || result.Project != "example" || result.ServerVersion != "6.0" || result.Instances == nil || len(result.Instances) != 0 {
		t.Fatalf("unexpected result %s: %v", stdout.String(), err)
	}
}

func TestInspectProxmoxCommand(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "PVEAPIToken=user@realm!test=secret" {
			t.Error("missing token")
		}
		switch r.URL.Path {
		case "/api2/json/version":
			fmt.Fprint(w, `{"data":{"version":"8.4"}}`)
		case "/api2/json/cluster/resources":
			fmt.Fprint(w, `{"data":[]}`)
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	defer server.Close()
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHIPPIN_PROXMOX_TOKEN_ID", "user@realm!test")
	t.Setenv("SHIPPIN_PROXMOX_TOKEN_SECRET", "secret")
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), []string{"inspect-proxmox", "-endpoint", server.URL, "-ca-file", caFile}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Provider  string `json:"provider"`
		Instances []any  `json:"instances"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || got.Provider != "proxmox" || got.Instances == nil {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
	t.Setenv("SHIPPIN_PROXMOX_TOKEN_SECRET", "")
	stdout.Reset()
	if err := run(context.Background(), []string{"inspect-proxmox", "-endpoint", server.URL}, &stdout, &stderr); err == nil || stdout.Len() != 0 {
		t.Fatal("missing token accepted or output emitted")
	}
}

func TestCommandRejectsMissingArgumentsWithoutOutput(t *testing.T) {
	for _, args := range [][]string{nil, {"inspect-incus"}, {"inspect-incus", "-socket", "relative", "-project", "default"}, {"other"}} {
		var stdout, stderr bytes.Buffer
		if err := run(context.Background(), args, &stdout, &stderr); err == nil || stdout.Len() != 0 {
			t.Fatalf("args %v: expected error and no output, got %v, %q", args, err, stdout.String())
		}
	}
}
