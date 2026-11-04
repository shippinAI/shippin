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
			Type   string `json:"type"`
			Name   string `json:"name"`
			Online int    `json:"online"`
			Local  int    `json:"local"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &r) != nil {
		return
	}
	for _, item := range r.Data {
		switch item.Type {
		case "cluster":
			d.Cluster = item.Name
		case "node":
			d.Members = append(d.Members, hv.ClusterMember{Name: item.Name, Online: item.Online == 1, Local: item.Local == 1})
		}
	}
	sort.Slice(d.Members, func(i, j int) bool { return d.Members[i].Name < d.Members[j].Name })
}
