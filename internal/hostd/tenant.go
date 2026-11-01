package hostd

import (
	"context"
	"fmt"
)

type ProvisionResult struct {
	Project         string `json:"project"`
	Pool            string `json:"pool"`
	Network         string `json:"network"`
	Dataset         string `json:"dataset"`
	SSHIP           string `json:"ssh_ip"`
	RestartRequired bool   `json:"restart_required,omitempty"`
}

// Provision creates (idempotently) the tenant silo from 07/07a/07b:
// encrypted dataset → pool → restricted project → OVN network (transit uplink) → hub peering + ACL → profile.
func (s *Server) Provision(ctx context.Context, t, planName string) (*ProvisionResult, error) {
	plan, ok := s.cfg.Plans[planName]
	if !ok {
		return nil, fmt.Errorf("unknown plan")
	}
	ds := Dataset(s.cfg.DatasetRoot, t)
	if _, err := run(ctx, "", "zfs", "list", "-H", "-o", "name", ds); err != nil {
		key, err := s.storeKey(ctx, t)
		if err != nil {
			return nil, err
		}
		if _, err := run(ctx, key, "zfs", "create", "-o", "encryption=aes-256-gcm", "-o", "keyformat=hex",
			"-o", "keylocation=prompt", "-o", fmt.Sprintf("quota=%dG", plan.QuotaGiB),
			"-o", "shippin:tenant="+t, ds); err != nil {
			return nil, err
		}
	}
	if !exists(ctx, "storage", "show", Pool(t)) {
		if _, err := incus(ctx, "storage", "create", Pool(t), "zfs", "source="+ds+"/incus",
			"volume.zfs.use_refquota=true", "volume.zfs.remove_snapshots=false"); err != nil {
			return nil, err
		}
	}
	if err := s.project(ctx, t, planName, plan); err != nil {
		return nil, err
	}
	created, err := s.tenantNetwork(ctx, t)
	if err != nil {
		return nil, err
	}
	// Roll back network-level pieces made by this attempt; dataset/pool/project are kept
	// (empty, idempotent) so a retry resumes cleanly.
	fail := func(err error) (*ProvisionResult, error) {
		if created {
			_, _ = incus(ctx, "profile", "device", "remove", "default", "eth0", "--project="+Project(t))
			_ = s.dropNetwork(ctx, t)
		}
		return nil, err
	}
	if err := s.connectHub(ctx, t); err != nil {
		return fail(err)
	}
	if err := s.profile(ctx, t, plan); err != nil {
		return fail(err)
	}
	if m, _ := s.tenantMode(ctx, t); m.Mode == modeVM {
		if err := s.EnsureTenantVM(ctx, t, plan); err != nil {
			return fail(fmt.Errorf("tenant VM: %w", err))
		}
	}
	if err := s.growToPlan(ctx, t, plan); err != nil {
		return fail(fmt.Errorf("storage: %w", err))
	}
	restart, err := s.syncVMShape(ctx, t, plan)
	if err != nil {
		return fail(fmt.Errorf("vm shape: %w", err))
	}
	if s.cfg.StarterDB.Enabled {
		if _, err := s.EnsureStarterDB(ctx, t); err != nil {
			return nil, err // dataset/network kept: provisioning is idempotent, a retry resumes
		}
	}
	ip := s.cfg.Edge.PublicIP
	return &ProvisionResult{Project(t), Pool(t), Network(t), ds, ip, restart}, nil
}

func (s *Server) project(ctx context.Context, t, planName string, p Plan) error {
	if !planNameRe.MatchString(planName) {
		return fmt.Errorf("invalid plan")
	}
	cfg := []string{
		"features.images=false", "features.profiles=true", "features.storage.volumes=true", "features.networks=true",
		"restricted=true", "restricted.containers.privilege=isolated", "restricted.containers.nesting=block",
		"restricted.containers.lowlevel=block", "restricted.containers.interception=block",
		"restricted.virtual-machines.lowlevel=block", "restricted.devices.disk=managed", "restricted.devices.nic=managed",
		"restricted.devices.proxy=block", "restricted.devices.gpu=block", "restricted.devices.pci=block",
		"restricted.devices.usb=block", "restricted.devices.unix-char=block", "restricted.devices.unix-block=block",
		"restricted.devices.unix-hotplug=block", "restricted.networks.uplinks=" + s.cfg.Transit,
		"restricted.networks.access=" + Network(t), "restricted.snapshots=allow",
		fmt.Sprintf("limits.instances=%d", p.Instances), fmt.Sprintf("limits.cpu=%d", p.CPU),
		"limits.memory=" + p.Memory, "limits.networks=1",
		fmt.Sprintf("limits.disk.pool.%s=%dGiB", Pool(t), p.QuotaGiB), "user.shippin.tenant=" + t,
		planKey + "=" + planName,
	}
	for _, hp := range s.cfg.HiddenPools {
		cfg = append(cfg, "limits.disk.pool."+hp+"=0")
	}
	if !exists(ctx, "project", "show", Project(t)) {
		args := []string{"project", "create", Project(t)}
		if s.cfg.VMFirst && p.VM != nil {
			args = append(args, "-c", modeKey+"="+modeVM) // new tenants are VM-backed
		}
		if _, err := incus(ctx, args...); err != nil {
			return err
		}
	}
	cfg = append(cfg, "limits.virtual-machines="+s.vmLimit(ctx, t))
	_, err := incus(ctx, append([]string{"project", "set", Project(t)}, cfg...)...)
	return err
}

func (s *Server) profile(ctx context.Context, t string, p Plan) error {
	pr := "--project=" + Project(t)
	if err := ignoreExists(incus(ctx, "profile", "device", "add", "default", "root", "disk", pr,
		"path=/", "pool="+Pool(t), fmt.Sprintf("size=%dGiB", p.RootGiB))); err != nil {
		return err
	}
	if err := ignoreExists(incus(ctx, "profile", "device", "add", "default", "eth0", "nic", pr,
		"network="+Network(t), "name=eth0")); err != nil {
		return err
	}
	_, err := incus(ctx, "profile", "set", "default", pr, fmt.Sprintf("limits.cpu=%d", p.WorkspaceCPU),
		"limits.memory="+p.WorkspaceMem, "security.idmap.isolated=true")
	return err
}

// vmLimit: 1 for a VM-backed tenant (its tenant VM), 0 otherwise.
func (s *Server) vmLimit(ctx context.Context, t string) string {
	if m, _ := s.tenantMode(ctx, t); m.Mode == modeVM {
		return "1"
	}
	return "0"
}
