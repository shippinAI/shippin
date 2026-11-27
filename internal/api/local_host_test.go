package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/infrastructure/servers"
)

func envMap(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func fileMap(m map[string]string) func(string) ([]byte, error) {
	return func(p string) ([]byte, error) {
		if v, ok := m[p]; ok {
			return []byte(v), nil
		}
		return nil, errors.New("no such file")
	}
}

// The environment names the local Incus socket and the local Proxmox token; nothing named, nothing attached.
func TestLocalHostsFromEnv(t *testing.T) {
	none, err := localHostsFromEnv(envMap(nil), fileMap(nil), "m1")
	if err != nil || len(none) != 0 {
		t.Fatalf("nothing named: %v %v", none, err)
	}
	hosts, err := localHostsFromEnv(envMap(map[string]string{
		"SHIPPIN_LOCAL_INCUS_SOCKET":       "/var/lib/incus/unix.socket",
		"SHIPPIN_LOCAL_PROXMOX_TOKEN_ID":   "shippin@pve!panel",
		"SHIPPIN_LOCAL_PROXMOX_TOKEN_FILE": "/etc/shippin/proxmox-token",
		"SHIPPIN_LOCAL_PROXMOX_CERT_FILE":  "/etc/pve/local/pve-ssl.pem",
	}), fileMap(map[string]string{"/etc/shippin/proxmox-token": "s3cret\n", "/etc/pve/local/pve-ssl.pem": "PEM"}), "m1")
	if err != nil || len(hosts) != 2 {
		t.Fatalf("%v %v", hosts, err)
	}
	inc, pve := hosts[0], hosts[1]
	if inc.ep.Kind != "incus" || inc.ep.BaseURL != "unix:///var/lib/incus/unix.socket" || inc.ep.Slug != localIncusSlug || inc.ep.Label != "m1" {
		t.Errorf("incus: %+v", inc.ep)
	}
	if pve.ep.Kind != "proxmox" || pve.ep.BaseURL != "https://127.0.0.1:8006" || pve.ep.TokenID != "shippin@pve!panel" || pve.ep.TrustedCert != "PEM" || pve.secret != "s3cret" {
		t.Errorf("proxmox: %+v %q", pve.ep, pve.secret)
	}
	guest, err := localHostsFromEnv(envMap(map[string]string{"SHIPPIN_LOCAL_INCUS_TRUST_TOKEN_FILE": "/etc/shippin/incus-trust-token"}),
		fileMap(map[string]string{"/etc/shippin/incus-trust-token": "tok\n"}), "vm1")
	if err != nil || len(guest) != 1 || guest[0].trust != "tok" || guest[0].ep.Kind != "incus" || guest[0].ep.Slug != hostIncusSlug || guest[0].ep.Label != "Incus host" || guest[0].ep.BaseURL != "" {
		t.Errorf("host of a guest: %+v %v", guest, err)
	}
	for name, env := range map[string]map[string]string{
		"relative socket": {"SHIPPIN_LOCAL_INCUS_SOCKET": "incus.socket"},
		"no trust token":  {"SHIPPIN_LOCAL_INCUS_TRUST_TOKEN_FILE": "/nope"},
		"token, no files": {"SHIPPIN_LOCAL_PROXMOX_TOKEN_ID": "shippin@pve!panel"},
		"missing secret":  {"SHIPPIN_LOCAL_PROXMOX_TOKEN_ID": "x", "SHIPPIN_LOCAL_PROXMOX_TOKEN_FILE": "/nope", "SHIPPIN_LOCAL_PROXMOX_CERT_FILE": "/etc/pve/local/pve-ssl.pem"},
	} {
		if _, err := localHostsFromEnv(envMap(env), fileMap(map[string]string{"/etc/pve/local/pve-ssl.pem": "PEM"}), "m1"); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// fakeIncusSocket serves the Incus API's health answer on a Unix socket, trusted or not.
func fakeIncusSocket(t *testing.T, auth string) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "incus.socket")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("unix sockets are not available here: %v", err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"type":"sync","metadata":{"auth":"` + auth + `","environment":{"server_version":"6.0"}}}`))
	})}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	return filepath.ToSlash(sock)
}

// The local Incus is attached through its socket, once; one that does not trust the panel is not saved.
func TestAttachLocalIncus(t *testing.T) {
	dir := &memHV{}
	a := infraAPI(t, "", dir)
	h := localHost{ep: servers.Endpoint{Kind: "incus", BaseURL: "unix://" + fakeIncusSocket(t, "trusted"), Slug: localIncusSlug, Label: "m1"}}
	if err := a.attachLocalHost(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	if err := a.attachLocalHost(context.Background(), h); !errors.Is(err, servers.ErrSlugTaken) {
		t.Fatalf("a second start attached it again: %v", err)
	}
	if len(dir.items) != 1 || dir.items[0].Slug != localIncusSlug || dir.items[0].Kind != "incus" || dir.items[0].ID == "" || dir.items[0].Site == "" {
		t.Fatalf("saved: %+v", dir.items)
	}

	refused := &memHV{}
	b := infraAPI(t, "", refused)
	h.ep.BaseURL = "unix://" + fakeIncusSocket(t, "untrusted")
	if err := b.attachLocalHost(context.Background(), h); err == nil || len(refused.items) != 0 {
		t.Fatalf("an Incus that does not trust the panel was saved: %v %+v", err, refused.items)
	}
}

type answeringDriver struct{ hv.Driver }

func (answeringDriver) Kind() string { return "proxmox" }
func (answeringDriver) Health(context.Context) (hv.Health, error) {
	return hv.Health{Online: true}, nil
}
func (answeringDriver) Nodes(context.Context) ([]string, error) { return []string{"pve1"}, nil }

// The local Proxmox token's secret is stored sealed to the host, never as written, and the node is found.
func TestAttachLocalProxmoxSealsItsSecret(t *testing.T) {
	dir := &memHV{}
	a := infraAPI(t, "", dir)
	a.cfg.SessionKey = strings.Repeat("ab", 32)
	var seen servers.Endpoint
	a.openHV = func(ep servers.Endpoint) (hv.Driver, error) { seen = ep; return answeringDriver{}, nil }
	h := localHost{secret: "s3cret", ep: servers.Endpoint{Kind: "proxmox", BaseURL: "https://127.0.0.1:8006", TokenID: "shippin@pve!panel", Slug: localProxmoxSlug, Label: "m1"}}
	if err := a.attachLocalHost(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	if len(dir.items) != 1 {
		t.Fatalf("saved: %+v", dir.items)
	}
	got := dir.items[0]
	if got.Node != "pve1" || got.Secret != "" || len(got.SealedSecret) == 0 || strings.Contains(string(got.SealedSecret), "s3cret") {
		t.Fatalf("stored %+v", got)
	}
	s, _ := a.sealer()
	if plain, err := s.Open(got.SealedSecret, got.ID); err != nil || string(plain) != "s3cret" {
		t.Fatalf("the sealed secret does not open to the token: %q %v", plain, err)
	}
	if seen.Kind != "proxmox" {
		t.Fatalf("the host was not checked before it was saved: %+v", seen)
	}
}
