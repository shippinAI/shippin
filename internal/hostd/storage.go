package hostd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// CoworkDiskGiB is Cowork's QEMU disk (Anthropic's documented workspace image, 2026-10-22).
// It has to live on the org data volume, never on the VM root. No desktop is installed by this code.
const CoworkDiskGiB = 25

// lowSpacePercent is the free fraction of the org dataset that raises low_space.
// Proposal for the operator: 10 percent. Plans with cowork also raise it when the data volume
// has under CoworkDiskGiB free.
const lowSpacePercent = 10

var planNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

// StorageTarget is an absolute size. Zero fields are left as they are. Shrinking is refused.
type StorageTarget struct {
	QuotaGiB int `json:"quota_gib"`
	RootGiB  int `json:"root_gib"`
	DataGiB  int `json:"data_gib"`
}

// StorageReport is the org dataset plus the tenant VM disks that sit on it.
type StorageReport struct {
	Dataset         string `json:"dataset"`
	Pool            string `json:"pool"`
	Mode            string `json:"mode"`
	Plan            string `json:"plan,omitempty"`
	QuotaBytes      int64  `json:"quota_bytes"`
	UsedBytes       int64  `json:"used_bytes"`
	FreeBytes       int64  `json:"free_bytes"`
	RootGiB         int    `json:"root_gib,omitempty"`
	DataGiB         int    `json:"data_gib,omitempty"`
	DataUsed        int64  `json:"data_used_bytes,omitempty"`
	DataTotal       int64  `json:"data_total_bytes,omitempty"`
	LowSpace        bool   `json:"low_space"`
	LowReason       string `json:"low_space_reason,omitempty"`
	RestartRequired bool   `json:"restart_required,omitempty"`
	// CoworkDisk says where that disk will go. "org-data-volume" is the data zvol on p-<tenant>.
	// "none" means this tenant has no VM, so it has nowhere legal to put one.
	CoworkDisk string `json:"cowork_disk"`
}

func (s *Server) growToPlan(ctx context.Context, t string, p Plan) error {
	tgt := StorageTarget{QuotaGiB: p.QuotaGiB, RootGiB: p.RootGiB}
	if p.VM != nil {
		tgt.RootGiB = p.VM.RootGiB
		tgt.DataGiB = p.VM.DataGiB
	}
	_, err := s.GrowStorage(ctx, t, tgt)
	return err
}

// ApplyPlan records the plan and grows the org dataset and VM disks up to it.
// It does not shrink, does not restart the VM, and refuses to drop a VM that already exists.
func (s *Server) ApplyPlan(ctx context.Context, t, name string) (*StorageReport, error) {
	p, ok := s.cfg.Plans[name]
	if !ok || !planNameRe.MatchString(name) {
		return nil, fmt.Errorf("unknown plan")
	}
	m, err := s.tenantMode(ctx, t)
	if err != nil {
		return nil, err
	}
	if p.VM == nil && m.Mode == modeVM {
		return nil, fmt.Errorf("refusing to drop a tenant VM")
	}
	if p.VM != nil && m.Mode != modeVM {
		if _, err := incus(ctx, "project", "set", Project(t), modeKey+"="+modeVM); err != nil {
			return nil, err
		}
	}
	res, err := s.Provision(ctx, t, name)
	if err != nil {
		return nil, err
	}
	rep, err := s.StorageReport(ctx, t)
	if err != nil {
		return nil, err
	}
	rep.RestartRequired = res.RestartRequired
	return rep, nil
}

// StorageReport reads the org dataset quota and, for a VM tenant, the root and data disks.
func (s *Server) StorageReport(ctx context.Context, t string) (*StorageReport, error) {
	if s.cfg.DatasetRoot == "" {
		return nil, fmt.Errorf("dataset_root is not set")
	}
	ds := Dataset(s.cfg.DatasetRoot, t)
	m, err := s.tenantMode(ctx, t)
	if err != nil {
		return nil, err
	}
	quota, err := zfsBytes(ctx, "quota", ds)
	if err != nil {
		return nil, err
	}
	used, err := zfsBytes(ctx, "used", ds)
	if err != nil {
		return nil, err
	}
	rep := &StorageReport{
		Dataset: ds, Pool: Pool(t), Mode: "legacy", Plan: m.Plan,
		QuotaBytes: quota, UsedBytes: used, CoworkDisk: "none",
	}
	if quota > used {
		rep.FreeBytes = quota - used
	}
	if m.Mode != modeVM {
		if root, err := s.profileRootGiB(ctx, t); err == nil {
			rep.RootGiB = root
		}
	}
	if m.Mode == modeVM {
		rep.Mode = modeVM
		rep.CoworkDisk = "org-data-volume"
		root, data, dataUsed, dataTotal, err := s.vmDisks(ctx, t)
		if err != nil {
			return nil, err
		}
		rep.RootGiB, rep.DataGiB = root, data
		rep.DataUsed, rep.DataTotal = dataUsed, dataTotal
	}
	cowork := false
	if p, ok := s.cfg.Plans[m.Plan]; ok {
		cowork = p.Cowork
	}
	rep.LowSpace, rep.LowReason = lowSpace(quota, used, rep.DataTotal, rep.DataUsed, cowork)
	return rep, nil
}

// GrowStorage raises the org quota, the project pool limit, and (for a VM) the root disk,
// the data volume, and the inner btrfs. Every byte stays on p-<tenant> / tank/tenants/<tenant>.
func (s *Server) GrowStorage(ctx context.Context, t string, tgt StorageTarget) (*StorageReport, error) {
	if tgt.QuotaGiB < 0 || tgt.RootGiB < 0 || tgt.DataGiB < 0 {
		return nil, fmt.Errorf("sizes must be positive")
	}
	if tgt.QuotaGiB == 0 && tgt.RootGiB == 0 && tgt.DataGiB == 0 {
		return nil, fmt.Errorf("set quota_gib, root_gib, or data_gib")
	}
	cur, err := s.StorageReport(ctx, t)
	if err != nil {
		return nil, err
	}
	if tgt.DataGiB > 0 && cur.Mode != modeVM {
		return nil, fmt.Errorf("data volume belongs to the tenant VM")
	}
	newQuota, newRoot, newData := gibOf(cur.QuotaBytes), cur.RootGiB, cur.DataGiB
	if tgt.QuotaGiB > 0 {
		newQuota = tgt.QuotaGiB
	}
	if tgt.RootGiB > 0 {
		newRoot = tgt.RootGiB
	}
	if tgt.DataGiB > 0 {
		newData = tgt.DataGiB
	}
	if tgt.QuotaGiB > 0 && cur.QuotaBytes > 0 && int64(tgt.QuotaGiB)<<30 < cur.QuotaBytes {
		return nil, fmt.Errorf("shrinking storage is not supported")
	}
	if tgt.RootGiB > 0 && cur.RootGiB > 0 && tgt.RootGiB < cur.RootGiB {
		return nil, fmt.Errorf("shrinking storage is not supported")
	}
	if tgt.DataGiB > 0 && cur.DataGiB > 0 && tgt.DataGiB < cur.DataGiB {
		return nil, fmt.Errorf("shrinking storage is not supported")
	}
	if cur.Mode == modeVM && newQuota < newRoot+newData {
		return nil, fmt.Errorf("quota must cover the VM root plus the data volume")
	}
	if cur.Mode != modeVM && newRoot > 0 && newQuota < newRoot {
		return nil, fmt.Errorf("quota must cover the root disk")
	}
	if cur.UsedBytes > int64(newQuota)<<30 {
		return nil, fmt.Errorf("quota is below what the org already uses")
	}
	grewDisk := false
	if tgt.QuotaGiB > 0 && (cur.QuotaBytes == 0 || int64(tgt.QuotaGiB)<<30 > cur.QuotaBytes) {
		if _, err := runner(ctx, "", "zfs", "set", fmt.Sprintf("quota=%dG", tgt.QuotaGiB), cur.Dataset); err != nil {
			return nil, err
		}
		// The project may use only its own pool. Hidden pools stay at 0 from provision.
		if _, err := incus(ctx, "project", "set", Project(t), fmt.Sprintf("limits.disk.pool.%s=%dGiB", Pool(t), tgt.QuotaGiB)); err != nil {
			return nil, err
		}
	}
	if cur.Mode == modeVM {
		if tgt.RootGiB > cur.RootGiB {
			if _, err := incus(ctx, "config", "device", "set", TenantVM(t), "root", fmt.Sprintf("size=%dGiB", tgt.RootGiB), "--project="+Project(t)); err != nil {
				return nil, err
			}
			grewDisk = true
		}
		if tgt.DataGiB > cur.DataGiB {
			if _, err := incus(ctx, "storage", "volume", "set", Pool(t), TenantVMData(t), fmt.Sprintf("size=%dGiB", tgt.DataGiB), "--project="+Project(t)); err != nil {
				return nil, err
			}
			grewDisk = true
		}
		if grewDisk {
			if err := s.resizeGuest(ctx, t); err != nil {
				return nil, err
			}
		}
	} else if tgt.RootGiB > cur.RootGiB {
		if err := s.growLegacyRoots(ctx, t, tgt.RootGiB); err != nil {
			return nil, err
		}
	}
	return s.StorageReport(ctx, t)
}

// syncVMShape updates the tenant VM's CPU and memory to the plan without restarting it.
// A change returns restart=true: Incus keeps the running shape until the next boot.
func (s *Server) syncVMShape(ctx context.Context, t string, p Plan) (bool, error) {
	if p.VM == nil {
		return false, nil
	}
	m, err := s.tenantMode(ctx, t)
	if err != nil || m.Mode != modeVM {
		return false, err
	}
	var inst struct {
		Config map[string]string `json:"config"`
	}
	if err := query(ctx, fmt.Sprintf("/1.0/instances/%s?project=%s", TenantVM(t), Project(t)), &inst); err != nil {
		return false, nil
	}
	wantCPU, wantMem := strconv.Itoa(p.VM.CPU), p.VM.Memory
	if inst.Config["limits.cpu"] == wantCPU && inst.Config["limits.memory"] == wantMem {
		return false, nil
	}
	_, err = incus(ctx, "config", "set", TenantVM(t), "--project="+Project(t), "limits.cpu="+wantCPU, "limits.memory="+wantMem)
	return err == nil, err
}

func (s *Server) vmDisks(ctx context.Context, t string) (root, data int, dataUsed, dataTotal int64, err error) {
	var inst struct {
		ExpandedDevices map[string]map[string]string `json:"expanded_devices"`
	}
	if err = query(ctx, fmt.Sprintf("/1.0/instances/%s?project=%s", TenantVM(t), Project(t)), &inst); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("tenant VM not found")
	}
	root, err = parseGiB(inst.ExpandedDevices["root"]["size"])
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("vm root size: %w", err)
	}
	var vol struct {
		Config map[string]string `json:"config"`
	}
	if err = query(ctx, fmt.Sprintf("/1.0/storage-pools/%s/volumes/custom/%s?project=%s", Pool(t), TenantVMData(t), Project(t)), &vol); err != nil {
		return 0, 0, 0, 0, fmt.Errorf("data volume not found")
	}
	data, err = parseGiB(vol.Config["size"])
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("data volume size: %w", err)
	}
	at, err := s.newPlace(ctx, t)
	if err != nil || !at.vm {
		return root, data, 0, 0, err
	}
	var res struct {
		Space struct {
			Used  int64 `json:"used"`
			Total int64 `json:"total"`
		} `json:"space"`
	}
	if at.query(ctx, "/1.0/storage-pools/"+InnerPool+"/resources", &res) == nil {
		dataUsed, dataTotal = res.Space.Used, res.Space.Total
	}
	return root, data, dataUsed, dataTotal, nil
}

func (s *Server) resizeGuest(ctx context.Context, t string) error {
	if s.cfg.TenantVMInit == "" {
		return fmt.Errorf("tenant_vm_init is required to grow filesystems inside the tenant VM")
	}
	pr, vm := "--project="+Project(t), TenantVM(t)
	if _, err := incus(ctx, "file", "push", s.cfg.TenantVMInit, vm+"/usr/local/sbin/shippin-tvm-init", pr, "--mode=0755", "--uid=0", "--gid=0"); err != nil {
		return err
	}
	_, err := incus(ctx, "exec", vm, pr, "--", "/usr/local/sbin/shippin-tvm-init", "grow", t)
	return err
}

func (s *Server) growLegacyRoots(ctx context.Context, t string, gib int) error {
	pr := "--project=" + Project(t)
	cur, err := s.profileRootGiB(ctx, t)
	if err != nil {
		return err
	}
	if gib < cur {
		return fmt.Errorf("shrinking storage is not supported")
	}
	if gib > cur {
		if _, err := incus(ctx, "profile", "device", "set", "default", "root", fmt.Sprintf("size=%dGiB", gib), pr); err != nil {
			return err
		}
	}
	var list []struct {
		Name string `json:"name"`
	}
	if err := query(ctx, "/1.0/instances?project="+Project(t), &list); err != nil {
		return err
	}
	for _, i := range list {
		if i.Name == TenantVM(t) || !instanceNameRe.MatchString(i.Name) {
			continue
		}
		var inst struct {
			ExpandedDevices map[string]map[string]string `json:"expanded_devices"`
		}
		if err := query(ctx, fmt.Sprintf("/1.0/instances/%s?project=%s", i.Name, Project(t)), &inst); err != nil {
			return err
		}
		size, err := parseGiB(inst.ExpandedDevices["root"]["size"])
		if err != nil {
			return err
		}
		if size > gib {
			return fmt.Errorf("shrinking storage is not supported")
		}
		if size > 0 && size < gib {
			if _, err := incus(ctx, "config", "device", "set", i.Name, "root", fmt.Sprintf("size=%dGiB", gib), pr); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Server) profileRootGiB(ctx context.Context, t string) (int, error) {
	var prof struct {
		Devices map[string]map[string]string `json:"devices"`
	}
	if err := query(ctx, fmt.Sprintf("/1.0/profiles/default?project=%s", Project(t)), &prof); err != nil {
		return 0, err
	}
	return parseGiB(prof.Devices["root"]["size"])
}

func (s *Server) annotateSpace(ctx context.Context, t string, v *VMView) {
	rep, err := s.StorageReport(ctx, t)
	if err != nil {
		v.LowSpace = true
		v.LowReason = "org quota could not be read"
		return
	}
	v.QuotaBytes, v.UsedBytes = rep.QuotaBytes, rep.UsedBytes
	// Prefer the inner pool figures VMView already collected when the guest answered.
	dataUsed, dataTotal := rep.DataUsed, rep.DataTotal
	if v.DataTotal > 0 {
		dataUsed, dataTotal = v.DataUsed, v.DataTotal
	}
	cowork := false
	if p, ok := s.cfg.Plans[rep.Plan]; ok {
		cowork = p.Cowork
	}
	v.LowSpace, v.LowReason = lowSpace(rep.QuotaBytes, rep.UsedBytes, dataTotal, dataUsed, cowork)
}

func lowSpace(quota, used, dataTotal, dataUsed int64, cowork bool) (bool, string) {
	var reasons []string
	if quota > 0 {
		free := quota - used
		if free < 0 {
			free = 0
		}
		if free*100 < quota*int64(lowSpacePercent) {
			reasons = append(reasons, fmt.Sprintf("org dataset free space is under %d%%", lowSpacePercent))
		}
	}
	if cowork && dataTotal > 0 && dataTotal-dataUsed < int64(CoworkDiskGiB)<<30 {
		reasons = append(reasons, fmt.Sprintf("data volume has under %d GiB free; Cowork's disk has to fit there, not on the VM root", CoworkDiskGiB))
	}
	if len(reasons) == 0 {
		return false, ""
	}
	return true, strings.Join(reasons, "; ")
}

func zfsBytes(ctx context.Context, prop, ds string) (int64, error) {
	out, err := runner(ctx, "", "zfs", "get", "-Hp", "-o", "value", prop, ds)
	if err != nil {
		return 0, err
	}
	v := strings.TrimSpace(out)
	if v == "none" || v == "-" || v == "" {
		return 0, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("zfs %s: %q", prop, v)
	}
	return n, nil
}

func parseGiB(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	for _, suf := range []string{"GiB", "GB", "G"} {
		if n, ok := strings.CutSuffix(s, suf); ok {
			v, err := strconv.Atoi(n)
			if err != nil || v < 0 {
				return 0, fmt.Errorf("bad size %q", s)
			}
			return v, nil
		}
	}
	return 0, fmt.Errorf("bad size %q", s)
}

func gibOf(b int64) int {
	if b <= 0 {
		return 0
	}
	return int(b >> 30)
}

// instanceNameRe limits which host instances a legacy grow will resize.
var instanceNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,40}$`)

// scratchFor is where a tenant's exported bytes are written. Tests replace it.
var scratchFor = datasetScratch

func (s *Server) scratch(ctx context.Context, t string) (string, error) {
	return scratchFor(ctx, s.cfg.DatasetRoot, t)
}

// datasetScratch is a private directory on the org dataset mount (tank/tenants/<tenant>/scratch).
// It refuses a mountpoint that is not that dataset, so a publish never lands in the host tmp.
func datasetScratch(ctx context.Context, root, tenant string) (string, error) {
	if ValidUUID(tenant) != nil || root == "" || strings.Contains(root, "..") {
		return "", fmt.Errorf("tenant scratch needs the org dataset")
	}
	ds := Dataset(root, tenant)
	out, err := runner(ctx, "", "zfs", "get", "-H", "-o", "value", "mountpoint", ds)
	mp := strings.TrimSpace(out)
	if err != nil || mp == "" || mp == "none" || mp == "legacy" || !filepath.IsAbs(mp) || filepath.Base(mp) != tenant {
		return "", fmt.Errorf("tenant dataset is not mounted on the org path")
	}
	base := filepath.Join(mp, "scratch")
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", err
	}
	return os.MkdirTemp(base, "shippin-")
}
