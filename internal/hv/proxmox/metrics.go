package proxmox

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"shippin.cloud/console/internal/hv"
)

// rrdFresh is how old the newest averaged network point may be before it is not reported.
const rrdFresh = 5 * time.Minute

// HostMetrics reads the attached node's load from /nodes/{node}/status, and its network rate from the
// node's own one-minute averages (/nodes/{node}/rrddata). The status has no network counters; a token
// that may not read the averages leaves the network rates missing rather than failing the host.
func (c *Client) HostMetrics(ctx context.Context) (hv.HostMetrics, error) {
	node, err := c.attachedNode(ctx)
	if err != nil {
		return hv.HostMetrics{}, err
	}
	body, code, err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/status", nil)
	if err != nil {
		return hv.HostMetrics{}, err
	}
	if code != http.StatusOK {
		return hv.HostMetrics{}, fmt.Errorf("proxmox: node status HTTP %d: %s", code, clip(body))
	}
	var st struct {
		Data struct {
			CPU     *float64     `json:"cpu"`
			LoadAvg []flexNumber `json:"loadavg"`
			CPUInfo struct {
				CPUs int `json:"cpus"`
			} `json:"cpuinfo"`
			Memory *struct {
				Total int64 `json:"total"`
				Used  int64 `json:"used"`
			} `json:"memory"`
			RootFS *struct {
				Total int64 `json:"total"`
				Used  int64 `json:"used"`
			} `json:"rootfs"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &st); err != nil {
		return hv.HostMetrics{}, fmt.Errorf("proxmox: node status: %w", err)
	}
	m := hv.HostMetrics{Node: node, CPUs: st.Data.CPUInfo.CPUs, CPU: st.Data.CPU}
	if len(st.Data.LoadAvg) > 0 && st.Data.LoadAvg[0].ok {
		m.Load1 = hv.Ptr(st.Data.LoadAvg[0].v)
	}
	if mem := st.Data.Memory; mem != nil && mem.Total > 0 {
		m.MemoryUsed, m.MemoryTotal = hv.Ptr(mem.Used), hv.Ptr(mem.Total)
	}
	if fs := st.Data.RootFS; fs != nil && fs.Total > 0 {
		m.DiskUsed, m.DiskTotal = hv.Ptr(fs.Used), hv.Ptr(fs.Total)
	}
	m.NetInRate, m.NetOutRate = c.nodeNetRate(ctx, node)
	return m, nil
}

// nodeNetRate is the newest one-minute average of the node's network traffic, in bytes per second.
func (c *Client) nodeNetRate(ctx context.Context, node string) (in, out *float64) {
	body, code, err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/rrddata?timeframe=hour&cf=AVERAGE", nil)
	if err != nil || code != http.StatusOK {
		return nil, nil
	}
	var r struct {
		Data []struct {
			Time   int64    `json:"time"`
			NetIn  *float64 `json:"netin"`
			NetOut *float64 `json:"netout"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &r) != nil {
		return nil, nil
	}
	// Proxmox leaves a field out of a point it has no value for, often the newest one.
	for i := len(r.Data) - 1; i >= 0; i-- {
		p := r.Data[i]
		if p.NetIn == nil || p.NetOut == nil {
			continue
		}
		if c.now().Sub(time.Unix(p.Time, 0)) > rrdFresh {
			return nil, nil
		}
		return p.NetIn, p.NetOut
	}
	return nil, nil
}

// attachedNode is the node this endpoint names, or the cluster's first when it names none.
func (c *Client) attachedNode(ctx context.Context) (string, error) {
	if c.Endpoint.Node != "" {
		return c.Endpoint.Node, nil
	}
	nodes, err := c.Nodes(ctx)
	if err != nil {
		return "", err
	}
	if len(nodes) == 0 {
		return "", fmt.Errorf("proxmox: no nodes reported")
	}
	return nodes[0], nil
}

// GuestReadings reads every guest in the cluster from /cluster/resources, in one call. Proxmox
// reports CPU as a busy fraction of the guest's CPUs, and network as cumulative counters. It reports
// a virtual machine's used disk as 0 because it cannot see inside the guest's filesystem, so that
// value is left out for virtual machines; containers report it.
func (c *Client) GuestReadings(ctx context.Context) ([]hv.GuestReading, error) {
	body, code, err := c.do(ctx, http.MethodGet, "/cluster/resources?type=vm", nil)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("proxmox: resources HTTP %d: %s", code, clip(body))
	}
	var r struct {
		Data []struct {
			VMID     int      `json:"vmid"`
			Type     string   `json:"type"`
			Status   string   `json:"status"`
			Node     string   `json:"node"`
			Template int      `json:"template"`
			CPU      *float64 `json:"cpu"`
			MaxCPU   int      `json:"maxcpu"`
			Mem      *int64   `json:"mem"`
			MaxMem   *int64   `json:"maxmem"`
			Disk     *int64   `json:"disk"`
			MaxDisk  *int64   `json:"maxdisk"`
			NetIn    *int64   `json:"netin"`
			NetOut   *int64   `json:"netout"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("proxmox: resources: %w", err)
	}
	out := make([]hv.GuestReading, 0, len(r.Data))
	for _, g := range r.Data {
		if g.Template == 1 {
			continue
		}
		rd := hv.GuestReading{ID: strconv.Itoa(g.VMID), Node: g.Node, Status: g.Status, CPU: g.CPU, CPULimit: g.MaxCPU,
			MemoryUsed: g.Mem, MemoryTotal: positive(g.MaxMem), DiskTotal: positive(g.MaxDisk), NetIn: g.NetIn, NetOut: g.NetOut}
		if g.Type == "lxc" {
			rd.DiskUsed = g.Disk
		}
		out = append(out, rd)
	}
	return out, nil
}

func positive(v *int64) *int64 {
	if v == nil || *v <= 0 {
		return nil
	}
	return v
}

// flexNumber is a number Proxmox sends as a JSON number or as a string ("0.12").
type flexNumber struct {
	v  float64
	ok bool
}

func (n *flexNumber) UnmarshalJSON(raw []byte) error {
	s := strings.Trim(string(raw), `"`)
	v, err := strconv.ParseFloat(s, 64)
	if err == nil {
		n.v, n.ok = v, true
	}
	return nil
}
