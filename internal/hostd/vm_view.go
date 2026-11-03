package hostd

import (
	"context"
	"fmt"
	"strings"
)

type VMInner struct {
	Name      string `json:"name"`
	Workspace string `json:"workspace"`
	Status    string `json:"status"`
	IPv4      string `json:"ipv4"`
	MemoryB   int64  `json:"memory_bytes"`
	Processes int    `json:"processes"`
}

// VMView is the operator view of a tenant VM (sizes, usage, nested containers).
type VMView struct {
	Mode       string    `json:"mode"` // "vm" or "legacy"
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	IPv4       string    `json:"ipv4"`
	Inner      string    `json:"inner_subnet"`
	CPU        string    `json:"cpu_limit"`
	CPUNanos   int64     `json:"cpu_usage_ns"`
	MemLimit   string    `json:"memory_limit"`
	MemUsed    int64     `json:"memory_used_bytes"`
	MemTotal   int64     `json:"memory_total_bytes"`
	RootSize   string    `json:"root_size"`
	DataSize   string    `json:"data_size"`
	DataUsed   int64     `json:"data_used_bytes"`
	DataTotal  int64     `json:"data_total_bytes"`
	QuotaBytes int64     `json:"quota_bytes,omitempty"`
	UsedBytes  int64     `json:"used_bytes,omitempty"`
	LowSpace   bool      `json:"low_space"`
	LowReason  string    `json:"low_space_reason,omitempty"`
	Containers []VMInner `json:"containers"`
}

type vmState struct {
	Status string `json:"status"`
	CPU    struct {
		Usage int64 `json:"usage"`
	} `json:"cpu"`
	Memory struct {
		Usage int64 `json:"usage"`
		Total int64 `json:"total"`
	} `json:"memory"`
	Processes int `json:"processes"`
	Network   map[string]struct {
		Addresses []struct{ Family, Address, Scope string } `json:"addresses"`
	} `json:"network"`
}

// ipv4: the uplink address (containers name it eth0, the Debian VM enp5s0).
func (st vmState) ipv4() string {
	for _, nic := range []string{"eth0", "enp5s0"} {
		for _, a := range st.Network[nic].Addresses {
			if a.Family == "inet" && a.Scope == "global" {
				return a.Address
			}
		}
	}
	return ""
}

func (s *Server) VMView(ctx context.Context, t string) (*VMView, error) {
	m, err := s.tenantMode(ctx, t)
	if err != nil {
		return nil, err
	}
	v := &VMView{Mode: "legacy", Name: TenantVM(t), Inner: m.Inner, Containers: []VMInner{}}
	if m.Mode != modeVM {
		s.annotateSpace(ctx, t, v)
		return v, nil
	}
	v.Mode = modeVM
	var inst struct {
		Status         string                       `json:"status"`
		Config         map[string]string            `json:"config"`
		ExpandedDevice map[string]map[string]string `json:"expanded_devices"`
	}
	if err := query(ctx, fmt.Sprintf("/1.0/instances/%s?project=%s", TenantVM(t), Project(t)), &inst); err != nil {
		v.Status = "Missing"
		s.annotateSpace(ctx, t, v)
		return v, nil
	}
	v.Status, v.CPU, v.MemLimit = inst.Status, inst.Config["limits.cpu"], inst.Config["limits.memory"]
	v.RootSize = inst.ExpandedDevice["root"]["size"]
	var vol struct {
		Config map[string]string `json:"config"`
	}
	if query(ctx, fmt.Sprintf("/1.0/storage-pools/%s/volumes/custom/%s?project=%s", Pool(t), TenantVMData(t), Project(t)), &vol) == nil {
		v.DataSize = vol.Config["size"]
	}
	var st vmState
	if query(ctx, fmt.Sprintf("/1.0/instances/%s/state?project=%s", TenantVM(t), Project(t)), &st) == nil {
		v.CPUNanos, v.MemUsed, v.MemTotal, v.IPv4 = st.CPU.Usage, st.Memory.Usage, st.Memory.Total, st.ipv4()
	}
	if v.Status != "Running" {
		s.annotateSpace(ctx, t, v)
		return v, nil
	}
	s.innerView(ctx, t, v)
	s.annotateSpace(ctx, t, v)
	return v, nil
}

// innerView fills pool usage and nested containers; guest data is only decoded into typed fields.
func (s *Server) innerView(ctx context.Context, t string, v *VMView) {
	at, err := s.newPlace(ctx, t)
	if err != nil {
		return
	}
	var res struct {
		Space struct {
			Used  int64 `json:"used"`
			Total int64 `json:"total"`
		} `json:"space"`
	}
	if at.query(ctx, "/1.0/storage-pools/"+InnerPool+"/resources", &res) == nil {
		v.DataUsed, v.DataTotal = res.Space.Used, res.Space.Total
	}
	list, err := listInner(ctx, at, 2)
	if err != nil {
		return
	}
	for _, i := range list {
		ws := strings.TrimPrefix(i.Name, "w-")
		if ValidUUID(ws) != nil {
			ws = ""
		}
		ip := i.State.ipv4()
		if at.checkIP(ip) != nil {
			ip = ""
		}
		v.Containers = append(v.Containers, VMInner{i.Name, ws, i.Status, ip, i.State.Memory.Usage, i.State.Processes})
	}
}
