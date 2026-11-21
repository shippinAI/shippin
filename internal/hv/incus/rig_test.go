package incus_test

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
	"shippin.cloud/console/internal/runtime/runner"
)

// TestRigRunnerMachine makes a build runner's machine on a real Incus with the user data dispatch
// gives it, waits for its first boot to run the runner script and power it off, and removes it. It
// runs only against a throwaway rig (SHIPPIN_INCUS_RIG=https://host:port, and SHIPPIN_INCUS_RIG_DIR
// holding rig.crt and rig.key, a client certificate the rig trusts, and the rig's server.crt).
//
// It makes a container from a cloud-init image, because the local rig cannot run virtual machines;
// production runners are virtual machines, which this does not prove. The configuration is a dummy
// and the image has no GitHub runner, so the script finds nothing to run and powers off.
//
// SHIPPIN_INCUS_RIG_STEP=create keeps the machine for inspection; =delete only removes it.
func TestRigRunnerMachine(t *testing.T) {
	base, dir := os.Getenv("SHIPPIN_INCUS_RIG"), os.Getenv("SHIPPIN_INCUS_RIG_DIR")
	if base == "" || dir == "" {
		t.Skip("SHIPPIN_INCUS_RIG and SHIPPIN_INCUS_RIG_DIR not set")
	}
	step := os.Getenv("SHIPPIN_INCUS_RIG_STEP")
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
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	const name = "shr-0a1b2c3d4e5f-1"
	if step != "delete" {
		ud, err := runner.UserData("cmlnLXRlc3Qtbm90LWEtcmVhbC1jb25maWc=")
		if err != nil {
			t.Fatal(err)
		}
		g, err := c.CreateGuest(ctx, hv.GuestSpec{Kind: hv.KindCT, Name: name, Image: "debian/13/cloud", ImageServer: "https://images.linuxcontainers.org",
			ImageProtocol: "simplestreams", Cores: 1, UserData: ud, Start: true})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("made %s: %s", g.ID, g.Status)
		// The first boot runs the script, which powers the machine off: stopping by itself is the sign.
		for !strings.EqualFold(g.Status, "stopped") {
			if ctx.Err() != nil {
				t.Fatalf("%s did not power itself off: %s", name, g.Status)
			}
			time.Sleep(3 * time.Second)
			if g, err = c.Guest(ctx, name); err != nil {
				t.Fatal(err)
			}
		}
		t.Logf("%s powered itself off after its first boot", name)
		if step == "create" {
			return
		}
	}
	if err := c.DeleteGuest(ctx, name); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Guest(ctx, name); !errors.Is(err, hv.ErrGuestNotFound) {
		t.Fatalf("%s still there: %v", name, err)
	}
	t.Logf("%s removed", name)
}
