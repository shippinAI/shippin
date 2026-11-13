package runfleet

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProxmoxProvisionIsStub(t *testing.T) {
	_, err := Proxmox{}.Provision(context.Background(), Spec{})
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("err = %v", err)
	}
}

func TestProxmoxHealthProbe(t *testing.T) {
	var ping int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "PVEAPIToken=") {
			t.Errorf("missing token header")
		}
		if strings.HasSuffix(r.URL.Path, "/agent/ping") {
			ping++
			w.WriteHeader(http.StatusOK)
			return
		}
		if !strings.Contains(r.URL.Path, "/qemu/100/status/current") {
			t.Errorf("path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"status":"running","cpu":0.9,"mem":900,"maxmem":1000}}`))
	}))
	defer srv.Close()
	p := Proxmox{BaseURL: srv.URL, Token: "user@pam!id=secret", Node: "pve", Client: srv.Client()}
	sub, load := p.Health(context.Background(), Runner{Kind: "qemu", VMID: 100})
	if sub.State != HealthOK || !strings.Contains(sub.Detail, "guest agent alive") {
		t.Fatalf("substrate = %+v", sub)
	}
	if load.State != HealthWarn || ping != 1 {
		t.Fatalf("load = %+v ping %d", load, ping)
	}
	stopped, _ := p.Health(context.Background(), Runner{Kind: "qemu", VMID: 0})
	if stopped.State != HealthUnknown {
		t.Fatalf("missing guest = %+v", stopped)
	}
}

func TestIncusIsStub(t *testing.T) {
	_, err := Incus{}.Provision(context.Background(), Spec{})
	if err == nil || !strings.Contains(err.Error(), "incus") {
		t.Fatal(err)
	}
	sub, load := Incus{}.Health(context.Background(), Runner{})
	if sub.State != HealthUnknown || load.State != HealthUnknown {
		t.Fatalf("%+v %+v", sub, load)
	}
}
