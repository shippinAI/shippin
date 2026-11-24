package hostd

import (
	"fmt"
	"strings"
)

// innerRestrictions is the restriction set of every inner pj-<uuid> project. It mirrors the
// legacy "ws" project in deploy/image/tenant-vm/shippin-tvm-init; keep the two in sync.
func innerRestrictions(plan Plan) []string {
	kv := []string{
		"restricted=true", "restricted.containers.privilege=isolated",
		"restricted.containers.nesting=block", "restricted.containers.lowlevel=block",
		"restricted.containers.interception=block", "restricted.devices.disk=managed",
		"restricted.devices.nic=managed", "restricted.devices.proxy=block",
		"restricted.devices.gpu=block", "restricted.devices.pci=block", "restricted.devices.usb=block",
		"restricted.devices.unix-char=block", "restricted.devices.unix-block=block",
		"restricted.devices.unix-hotplug=block", "restricted.virtual-machines.lowlevel=block",
		"limits.virtual-machines=0", "restricted.networks.access=" + innerBridge,
	}
	if plan.Instances > 0 {
		kv = append(kv, fmt.Sprintf("limits.instances=%d", plan.Instances))
	}
	return kv
}

// innerInitArgs are the per-instance keys of a new workspace. In a pj- project the profile is
// the inner default project's (features.profiles=false), which has neither an isolated idmap nor
// plan limits, so both are set on the instance: the template size wins, the plan fills the rest.
func innerInitArgs(at place, plan Plan, z Size) []string {
	a := sizeArgs(z)
	if !at.vm || at.pj == "" {
		return a
	}
	a = append(a, "-c", "security.idmap.isolated=true")
	if z.CPU <= 0 && plan.WorkspaceCPU > 0 {
		a = append(a, "-c", fmt.Sprintf("limits.cpu=%d", plan.WorkspaceCPU))
	}
	if z.MemoryGiB <= 0 && strings.TrimSpace(plan.WorkspaceMem) != "" {
		a = append(a, "-c", "limits.memory="+plan.WorkspaceMem)
	}
	return a
}
