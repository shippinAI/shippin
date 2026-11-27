package incus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"shippin.cloud/console/internal/hv"
)

// readingsLimit bounds the instance listing with state, which grows with the number of guests.
const readingsLimit = 32 << 20

// HostMetrics reads the server's CPU count, memory and load average from /1.0/resources. That has no
// CPU usage, and the API calls used here give no host disk or network traffic (storage pools are not
// the host's disk), so those stay missing rather than being estimated. A server too old to report its
// load leaves that missing too.
func (c *Client) HostMetrics(ctx context.Context) (hv.HostMetrics, error) {
	var m hv.HostMetrics
	if body, code, err := c.do(ctx, http.MethodGet, "/1.0", nil); err == nil && code == http.StatusOK {
		var srv struct {
			Metadata struct {
				Environment struct {
					ServerName string `json:"server_name"`
				} `json:"environment"`
			} `json:"metadata"`
		}
		if json.Unmarshal(body, &srv) == nil {
			m.Node = srv.Metadata.Environment.ServerName
		}
	}
	body, code, err := c.do(ctx, http.MethodGet, "/1.0/resources", nil)
	if err != nil {
		return hv.HostMetrics{}, err
	}
	if code != http.StatusOK {
		return hv.HostMetrics{}, fmt.Errorf("incus: resources HTTP %d: %s", code, clip(body))
	}
	var r struct {
		Metadata struct {
			CPU struct {
				Total int `json:"total"`
			} `json:"cpu"`
			Memory struct {
				Total int64 `json:"total"`
				Used  int64 `json:"used"`
			} `json:"memory"`
			Load *struct {
				Average1Min float64 `json:"Average1Min"`
			} `json:"load"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return hv.HostMetrics{}, fmt.Errorf("incus: resources: %w", err)
	}
	if r.Metadata.Load != nil {
		m.Load1 = hv.Ptr(r.Metadata.Load.Average1Min)
	}
	m.CPUs = r.Metadata.CPU.Total
	if r.Metadata.Memory.Total > 0 {
		m.MemoryUsed, m.MemoryTotal = hv.Ptr(r.Metadata.Memory.Used), hv.Ptr(r.Metadata.Memory.Total)
	}
	return m, nil
}

// instanceState is the part of /1.0/instances/{name}/state the readings use.
type instanceState struct {
	CPU struct {
		Usage int64 `json:"usage"` // nanoseconds, cumulative
	} `json:"cpu"`
	Memory struct {
		Usage int64 `json:"usage"`
		Total int64 `json:"total"`
	} `json:"memory"`
	Disk map[string]struct {
		Usage int64 `json:"usage"`
		Total int64 `json:"total"`
	} `json:"disk"`
	Network map[string]struct {
		Type     string `json:"type"`
		Counters struct {
			BytesReceived int64 `json:"bytes_received"`
			BytesSent     int64 `json:"bytes_sent"`
		} `json:"counters"`
	} `json:"network"`
}

// GuestReadings reads every instance with its state in one call (/1.0/instances?recursion=2). Incus
// reports CPU as cumulative time, so the reading carries CPUSeconds and the CPUs the guest may use, and
// network as cumulative counters seen from inside the guest. A value Incus reports as 0 or less for a
// running guest's CPU time or memory means it could not read it (a virtual machine without its agent),
// so it is left out rather than stored as zero.
func (c *Client) GuestReadings(ctx context.Context) ([]hv.GuestReading, error) {
	body, code, err := c.doLimit(ctx, http.MethodGet, c.path("/1.0/instances", "recursion", "2"), nil, readingsLimit)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("incus: instances HTTP %d: %s", code, clip(body))
	}
	var r struct {
		Metadata []struct {
			Name           string            `json:"name"`
			Status         string            `json:"status"`
			Type           string            `json:"type"`
			Location       string            `json:"location"`
			ExpandedConfig map[string]string `json:"expanded_config"`
			State          *instanceState    `json:"state"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("incus: instances: %w", err)
	}
	out := make([]hv.GuestReading, 0, len(r.Metadata))
	for _, in := range r.Metadata {
		rd := hv.GuestReading{ID: in.Name, Status: in.Status, CPULimit: cpuLimit(in.ExpandedConfig["limits.cpu"], in.Type == "virtual-machine")}
		if in.Location != "none" {
			rd.Node = in.Location
		}
		if st := in.State; st != nil {
			readState(&rd, st)
		}
		out = append(out, rd)
	}
	return out, nil
}

func readState(rd *hv.GuestReading, st *instanceState) {
	if st.CPU.Usage > 0 {
		rd.CPUSeconds = hv.Ptr(float64(st.CPU.Usage) / 1e9)
	}
	if st.Memory.Usage > 0 {
		rd.MemoryUsed = hv.Ptr(st.Memory.Usage)
		if st.Memory.Total > 0 {
			rd.MemoryTotal = hv.Ptr(st.Memory.Total)
		}
	}
	if len(st.Disk) > 0 {
		var used, total int64
		allTotals := true
		for _, d := range st.Disk {
			used += max(d.Usage, 0)
			if d.Total > 0 {
				total += d.Total
			} else {
				allTotals = false
			}
		}
		rd.DiskUsed = hv.Ptr(used)
		if allTotals {
			rd.DiskTotal = hv.Ptr(total)
		}
	}
	var in, out int64
	seen := false
	for name, n := range st.Network {
		if name == "lo" || n.Type == "loopback" {
			continue
		}
		in, out, seen = in+n.Counters.BytesReceived, out+n.Counters.BytesSent, true
	}
	if seen {
		rd.NetIn, rd.NetOut = hv.Ptr(in), hv.Ptr(out)
	}
}

// cpuLimit is how many CPUs limits.cpu lets a guest use: a count ("2") or a set of CPUs ("0-3,6").
// Unset, a container may use all of the host's (0) and a virtual machine has Incus's default of one.
func cpuLimit(v string, vm bool) int {
	v = strings.TrimSpace(v)
	if v == "" {
		if vm {
			return 1
		}
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return n
	}
	count := 0
	for _, part := range strings.Split(v, ",") {
		lo, hi, isRange := strings.Cut(strings.TrimSpace(part), "-")
		a, errA := strconv.Atoi(lo)
		if !isRange {
			if errA != nil {
				return 0
			}
			count++
			continue
		}
		b, errB := strconv.Atoi(hi)
		if errA != nil || errB != nil || b < a {
			return 0
		}
		count += b - a + 1
	}
	return count
}
