package proxmox

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"shippin.cloud/console/internal/hv"
)

const maxReplicationJobs = 200

// apiBool accepts both JSON booleans and the 0/1 values emitted by some PVE
// API serializers for fields whose API schema is boolean.
type apiBool bool

func (b *apiBool) UnmarshalJSON(raw []byte) error {
	var value bool
	if json.Unmarshal(raw, &value) == nil {
		*b = apiBool(value)
		return nil
	}
	var number int
	if err := json.Unmarshal(raw, &number); err != nil || (number != 0 && number != 1) {
		return fmt.Errorf("expected boolean or 0/1")
	}
	*b = apiBool(number == 1)
	return nil
}

// Nodes lists the cluster's nodes, so an operator with one node does not have to name it.
func (c *Client) Nodes(ctx context.Context) ([]string, error) {
	body, code, err := c.do(ctx, http.MethodGet, "/nodes", nil)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("proxmox: nodes HTTP %d: %s", code, clip(body))
	}
	var r struct {
		Data []struct {
			Node string `json:"node"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("proxmox: nodes: %w", err)
	}
	out := make([]string, 0, len(r.Data))
	for _, n := range r.Data {
		out = append(out, n.Node)
	}
	sort.Strings(out)
	return out, nil
}

// Details reads the attached node and, when there is one, its cluster.
func (c *Client) Details(ctx context.Context) (hv.NodeDetails, error) {
	node := c.Endpoint.Node
	if node == "" {
		nodes, err := c.Nodes(ctx)
		if err != nil {
			return hv.NodeDetails{}, err
		}
		if len(nodes) == 0 {
			return hv.NodeDetails{}, fmt.Errorf("proxmox: no nodes reported")
		}
		node = nodes[0]
	}
	body, code, err := c.do(ctx, http.MethodGet, "/nodes/"+url.PathEscape(node)+"/status", nil)
	if err != nil {
		return hv.NodeDetails{}, err
	}
	if code != http.StatusOK {
		return hv.NodeDetails{}, fmt.Errorf("proxmox: node status HTTP %d: %s", code, clip(body))
	}
	var st struct {
		Data struct {
			PVEVersion string `json:"pveversion"`
			KVersion   string `json:"kversion"`
			Uptime     int64  `json:"uptime"`
			CPUInfo    struct {
				Model string `json:"model"`
				CPUs  int    `json:"cpus"`
			} `json:"cpuinfo"`
			Memory struct {
				Total int64 `json:"total"`
				Used  int64 `json:"used"`
			} `json:"memory"`
			CurrentKernel struct {
				Release string `json:"release"`
			} `json:"current-kernel"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &st); err != nil {
		return hv.NodeDetails{}, fmt.Errorf("proxmox: node status: %w", err)
	}
	d := hv.NodeDetails{
		Hostname: node, Release: st.Data.PVEVersion, Kernel: st.Data.CurrentKernel.Release, CPUModel: st.Data.CPUInfo.Model, CPUs: st.Data.CPUInfo.CPUs,
		MemTotal: st.Data.Memory.Total, MemUsed: st.Data.Memory.Used, Uptime: st.Data.Uptime,
	}
	// Disk inventory is best-effort: restricted provider tokens can still read node facts.
	// Skip SMART probes on overview reads; they are slower and are not required for capacity.
	d.Disks, _ = c.nodeDisks(ctx, node)
	if d.Kernel == "" {
		if f := strings.Fields(st.Data.KVersion); len(f) >= 2 {
			d.Kernel = f[1]
		}
	}
	c.addCluster(ctx, &d)
	c.addReplication(ctx, node, &d)
	return d, nil
}

func (c *Client) nodeDisks(ctx context.Context, node string) ([]hv.NodeDisk, error) {
	path := "/nodes/" + url.PathEscape(node) + "/disks/list?skipsmart=1"
	body, code, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("proxmox: node disks HTTP %d: %s", code, clip(body))
	}
	var result struct {
		Data []struct {
			DevPath string `json:"devpath"`
			Model   string `json:"model"`
			Size    int64  `json:"size"`
			Used    string `json:"used"`
			Mounted bool   `json:"mounted"`
			GPT     bool   `json:"gpt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("proxmox: node disks: %w", err)
	}
	disks := make([]hv.NodeDisk, 0, len(result.Data))
	for _, disk := range result.Data {
		if disk.DevPath == "" {
			continue
		}
		disks = append(disks, hv.NodeDisk{
			Path: disk.DevPath, Model: disk.Model, Size: disk.Size,
			Used: disk.Used, Mounted: disk.Mounted, GPT: disk.GPT,
		})
	}
	return disks, nil
}

// addCluster is best effort: a token that may read the node may not read the cluster.
func (c *Client) addCluster(ctx context.Context, d *hv.NodeDetails) {
	body, code, err := c.do(ctx, http.MethodGet, "/cluster/status", nil)
	if err != nil || code != http.StatusOK {
		return
	}
	var r struct {
		Data []struct {
			Type    string   `json:"type"`
			Name    string   `json:"name"`
			Online  apiBool  `json:"online"`
			Local   apiBool  `json:"local"`
			Quorate *apiBool `json:"quorate"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &r) != nil {
		return
	}
	for _, item := range r.Data {
		switch item.Type {
		case "cluster":
			d.Cluster = item.Name
			if item.Quorate != nil {
				quorate := bool(*item.Quorate)
				d.Quorate = &quorate
			}
		case "node":
			d.Members = append(d.Members, hv.ClusterMember{Name: item.Name, Online: bool(item.Online), Local: bool(item.Local)})
		}
	}
	sort.Slice(d.Members, func(i, j int) bool { return d.Members[i].Name < d.Members[j].Name })
}

// addReplication observes jobs local to the selected Proxmox node. VM.Audit
// permission is required by Proxmox; restricted tokens leave status unavailable.
func (c *Client) addReplication(ctx context.Context, node string, d *hv.NodeDetails) {
	available := false
	d.ReplicationStatusAvailable = &available
	path := "/nodes/" + url.PathEscape(node) + "/replication"
	body, code, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil || code != http.StatusOK {
		return
	}
	var result struct {
		Data []struct {
			ID        string `json:"id"`
			Guest     int    `json:"guest"`
			Target    string `json:"target"`
			Schedule  string `json:"schedule"`
			LastSync  int64  `json:"last_sync"`
			FailCount int    `json:"fail_count"`
			Error     string `json:"error"`
			PID       int    `json:"pid"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &result) != nil {
		return
	}
	available = true
	limit := len(result.Data)
	if limit > maxReplicationJobs {
		limit = maxReplicationJobs
	}
	d.ReplicationJobs = make([]hv.ReplicationJob, 0, limit)
	for _, job := range result.Data[:limit] {
		if job.ID == "" || job.Guest <= 0 {
			continue
		}
		d.ReplicationJobs = append(d.ReplicationJobs, hv.ReplicationJob{
			ID: job.ID, Guest: job.Guest, Target: job.Target, Schedule: job.Schedule,
			LastSync: job.LastSync, FailCount: job.FailCount, Error: job.Error, Running: job.PID > 0,
		})
	}
	sort.Slice(d.ReplicationJobs, func(i, j int) bool { return d.ReplicationJobs[i].ID < d.ReplicationJobs[j].ID })
}
