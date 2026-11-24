package hostd

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// EnsureTenantVM creates (idempotently) the tenant VM: data volume, VM on the tenant network with a
// static address and the inner /24 routed to it, then initialises the Incus inside it. Whatever this
// attempt created is rolled back on failure, so a retry starts clean.
func (s *Server) EnsureTenantVM(ctx context.Context, t string, plan Plan) (err error) {
	if plan.VM == nil || s.cfg.TenantVMImage == "" {
		return fmt.Errorf("plan has no tenant VM size")
	}
	v, pr, vm := plan.VM, "--project="+Project(t), TenantVM(t)
	inner, err := s.ensureInnerSubnet(ctx, t)
	if err != nil {
		return err
	}
	m, err := s.tenantMode(ctx, t)
	if err != nil {
		return err
	}
	addr, err := vmAddr(m.Net)
	if err != nil {
		return err
	}
	var madeVol, madeVM bool
	defer func() {
		if err == nil {
			return
		}
		log.Printf("tenant VM %s: rolling back after: %v", t, err)
		if madeVM {
			_, _ = incus(ctx, "delete", vm, pr, "--force")
		}
		if madeVol {
			_, _ = incus(ctx, "storage", "volume", "delete", Pool(t), TenantVMData(t), pr)
		}
	}()
	if !exists(ctx, "storage", "volume", "show", Pool(t), TenantVMData(t), pr) {
		if _, err = incus(ctx, "storage", "volume", "create", Pool(t), TenantVMData(t), pr, "--type=block",
			fmt.Sprintf("size=%dGiB", v.DataGiB), "user.shippin.tenant="+t); err != nil {
			return err
		}
		madeVol = true
	}
	if !exists(ctx, "info", vm, pr) {
		if _, err = incus(ctx, "init", s.cfg.TenantVMImage, vm, "--vm", pr,
			"-c", fmt.Sprintf("limits.cpu=%d", v.CPU), "-c", "limits.memory="+v.Memory,
			"-c", "user.shippin.tenant="+t, "-c", "user.shippin.role=tenant-vm",
			"-d", fmt.Sprintf("root,size=%dGiB", v.RootGiB),
			"-d", "eth0,ipv4.address="+addr, "-d", "eth0,ipv4.routes="+inner); err != nil {
			return err
		}
		madeVM = true
		if _, err = incus(ctx, "config", "device", "add", vm, "data", "disk", pr,
			"pool="+Pool(t), "source="+TenantVMData(t)); err != nil {
			return err
		}
	}
	if _, err = incus(ctx, "start", vm, pr); err != nil && !isRunning(err) {
		return err
	}
	if err = s.initInner(ctx, t, inner); err != nil {
		return err
	}
	// Default inner workspace size, same keys as the host tenant profile.
	_, err = place{t: t, vm: true}.do(ctx, "profile", "set", "default",
		fmt.Sprintf("limits.cpu=%d", plan.WorkspaceCPU), "limits.memory="+plan.WorkspaceMem)
	return err
}

// initInner waits for the VM agent, then runs shippin-tvm-init (idempotent) inside the VM.
func (s *Server) initInner(ctx context.Context, t, inner string) error {
	gw, err := innerGateway(inner)
	if err != nil {
		return err
	}
	if err := waitAgent(ctx, t); err != nil {
		return err
	}
	mtu := "1442"
	var n struct {
		Config map[string]string `json:"config"`
	}
	if query(ctx, fmt.Sprintf("/1.0/networks/%s?project=%s", Network(t), Project(t)), &n) == nil && n.Config["bridge.mtu"] != "" {
		mtu = n.Config["bridge.mtu"]
	}
	if s.cfg.TenantVMInit != "" {
		if _, err := incus(ctx, "file", "push", s.cfg.TenantVMInit, TenantVM(t)+"/usr/local/sbin/shippin-tvm-init",
			"--project="+Project(t), "--mode=0755", "--uid=0", "--gid=0"); err != nil {
			return err
		}
	}
	_, err = incus(ctx, "exec", TenantVM(t), "--project="+Project(t), "--", "/usr/local/sbin/shippin-tvm-init", gw, mtu, t)
	return err
}

func waitAgent(ctx context.Context, t string) error {
	restarts := 0
	for i := 0; i < 90; i++ {
		_, err := incus(ctx, "exec", TenantVM(t), "--project="+Project(t), "--", "true")
		if err == nil {
			return nil
		}
		// First boot: shim's fallback writes the UEFI boot entries and resets the guest; on an OVN NIC
		// Incus can turn that reset into a stop. Start it again (bounded) instead of failing the boot.
		if strings.Contains(err.Error(), "not running") && restarts < maxBootRestarts {
			restarts++
			log.Printf("tenant VM %s stopped during boot (guest reset); starting it again (%d/%d)", t, restarts, maxBootRestarts)
			if _, err := incus(ctx, "start", TenantVM(t), "--project="+Project(t)); err != nil && !isRunning(err) {
				log.Printf("tenant VM %s: restart: %v", t, err)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("tenant VM agent did not come up")
}

// maxBootRestarts bounds how often waitAgent restarts a VM that stopped while booting.
const maxBootRestarts = 3
