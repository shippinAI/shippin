package runner_test

import (
	"context"
	"crypto/tls"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/hv/incus"
	"shippin.cloud/console/internal/ports"
	"shippin.cloud/console/internal/runtime/runner"
)

// rigHosts is ports.Machines on a real Incus rig. It makes containers, because the local rig cannot
// run virtual machines; the panel's own port makes virtual machines.
type rigHosts struct{ c *incus.Client }

func (r rigHosts) Hosts(context.Context) ([]string, error) { return []string{host1}, nil }

func (r rigHosts) Create(ctx context.Context, _ string, m ports.Machine) (string, error) {
	g, err := r.c.CreateGuest(ctx, hv.GuestSpec{Kind: hv.KindCT, Name: m.Name, Image: "debian/13/cloud", ImageServer: "https://images.linuxcontainers.org",
		ImageProtocol: "simplestreams", Cores: m.Cores, UserData: m.UserData, Start: true})
	return g.ID, err
}

func (r rigHosts) Delete(ctx context.Context, _ string, guest string) error {
	err := r.c.DeleteGuest(ctx, guest)
	if errors.Is(err, hv.ErrGuestNotFound) {
		return nil
	}
	return err
}

// TestRigDispatch runs one job's machine through dispatch on a real Incus (the rig described in
// internal/hv/incus/rig_test.go), with a fake GitHub: started for the waiting job with its first-boot
// data, powering itself off once the runner script ran, and removed when the job ends.
func TestRigDispatch(t *testing.T) {
	base, dir := os.Getenv("SHIPPIN_INCUS_RIG"), os.Getenv("SHIPPIN_INCUS_RIG_DIR")
	if base == "" || dir == "" {
		t.Skip("SHIPPIN_INCUS_RIG and SHIPPIN_INCUS_RIG_DIR not set")
	}
	server, err := os.ReadFile(filepath.Join(dir, "server.crt"))
	if err != nil {
		t.Fatal(err)
	}
	cert, err := tls.LoadX509KeyPair(filepath.Join(dir, "rig.crt"), filepath.Join(dir, "rig.key"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := incus.New(incus.Endpoint{BaseURL: base, TrustedPEM: server, ClientCert: &cert}, func() (string, error) { return "", nil })
	if err != nil {
		t.Fatal(err)
	}
	w := newWorld(t, nil)
	w.d.Machines = rigHosts{c}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	job := w.queue(acme, 1)
	if out, err := runner.Provision(ctx, w.d, job, false); err != nil || out.Started == "" {
		t.Fatalf("provision: %+v %v", out, err)
	}
	m := w.machineFor(job)
	g, err := c.Guest(ctx, m.guest)
	if err != nil || m.state != "running" {
		t.Fatalf("%+v on Incus: %+v %v", m, g, err)
	}
	t.Logf("machine %s is %s on Incus as %s", m.id, g.Status, m.guest)
	for !strings.EqualFold(g.Status, "stopped") {
		if ctx.Err() != nil {
			t.Fatalf("%s did not power itself off: %s", m.guest, g.Status)
		}
		time.Sleep(3 * time.Second)
		if g, err = c.Guest(ctx, m.guest); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("%s powered itself off after its first boot", m.guest)

	_, ids := w.receive(acme, "completed", 1, m.guest, "success")
	if len(ids) != 1 || ids[0] != m.id {
		t.Fatalf("the ended job's machine: %v", ids)
	}
	if _, err := runner.Remove(ctx, w.d, m.id); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Guest(ctx, m.guest); !errors.Is(err, hv.ErrGuestNotFound) {
		t.Fatalf("%s is still on Incus: %v", m.guest, err)
	}
	if got := w.machineFor(job); got.state != "removed" {
		t.Fatalf("after removal: %+v", got)
	}
	t.Logf("%s removed from Incus; the machine is %s", m.guest, "removed")
}
