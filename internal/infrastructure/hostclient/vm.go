package hostclient

import (
	"context"
	"net/http"
)

// VMContainer is one workspace container nested in a tenant VM.
type VMContainer struct {
	Name      string `json:"name"`
	Workspace string `json:"workspace"`
	Status    string `json:"status"`
	IPv4      string `json:"ipv4"`
	MemoryB   int64  `json:"memory_bytes"`
	Processes int    `json:"processes"`
}

// TenantVM mirrors hostd's VMView (mode "vm" or "legacy").
type TenantVM struct {
	Mode       string        `json:"mode"`
	Name       string        `json:"name"`
	Status     string        `json:"status"`
	IPv4       string        `json:"ipv4"`
	Inner      string        `json:"inner_subnet"`
	CPU        string        `json:"cpu_limit"`
	CPUNanos   int64         `json:"cpu_usage_ns"`
	MemLimit   string        `json:"memory_limit"`
	MemUsed    int64         `json:"memory_used_bytes"`
	MemTotal   int64         `json:"memory_total_bytes"`
	RootSize   string        `json:"root_size"`
	DataSize   string        `json:"data_size"`
	DataUsed   int64         `json:"data_used_bytes"`
	DataTotal  int64         `json:"data_total_bytes"`
	Containers []VMContainer `json:"containers"`
}

func (c *Client) TenantVM(ctx context.Context, t string) (*TenantVM, error) {
	out := &TenantVM{}
	return out, c.do(ctx, http.MethodGet, "/tenants/"+t+"/vm", nil, out)
}

// EnableTenantVM makes an existing tenant VM-backed (its legacy workspaces stay where they are).
func (c *Client) EnableTenantVM(ctx context.Context, t, plan string) (*TenantVM, error) {
	out := &TenantVM{}
	return out, c.do(ctx, http.MethodPost, "/tenants/"+t+"/vm", map[string]string{"plan": plan}, out)
}

// StorageReport mirrors hostd's per-org disk report.
type StorageReport struct {
	Dataset    string `json:"dataset"`
	Pool       string `json:"pool"`
	Mode       string `json:"mode"`
	Plan       string `json:"plan,omitempty"`
	QuotaBytes int64  `json:"quota_bytes"`
	UsedBytes  int64  `json:"used_bytes"`
	FreeBytes  int64  `json:"free_bytes"`
	RootGiB    int    `json:"root_gib,omitempty"`
	DataGiB    int    `json:"data_gib,omitempty"`
	DataUsed   int64  `json:"data_used_bytes,omitempty"`
	DataTotal  int64  `json:"data_total_bytes,omitempty"`
	LowSpace   bool   `json:"low_space"`
	LowReason  string `json:"low_space_reason,omitempty"`
	Restart    bool   `json:"restart_required,omitempty"`
	CoworkDisk string `json:"cowork_disk"`
}

// TenantStorage reads the org dataset and VM disks.
func (c *Client) TenantStorage(ctx context.Context, t string) (*StorageReport, error) {
	out := &StorageReport{}
	return out, c.do(ctx, http.MethodGet, "/tenants/"+t+"/storage", nil, out)
}

// GrowStorage raises quota, root, and data. Zero fields are unchanged. Shrinking is refused.
func (c *Client) GrowStorage(ctx context.Context, t string, quota, root, data int) (*StorageReport, error) {
	out := &StorageReport{}
	body := map[string]int{"quota_gib": quota, "root_gib": root, "data_gib": data}
	return out, c.do(ctx, http.MethodPost, "/tenants/"+t+"/storage", body, out)
}

// ApplyPlan grows the tenant up to a plan. It does not shrink or restart the VM.
func (c *Client) ApplyPlan(ctx context.Context, t, plan string) (*StorageReport, error) {
	out := &StorageReport{}
	return out, c.do(ctx, http.MethodPost, "/tenants/"+t+"/plan", map[string]string{"plan": plan}, out)
}

// VMAction runs start, stop or restart on the tenant VM.
func (c *Client) VMAction(ctx context.Context, t, action string) error {
	return c.do(ctx, http.MethodPost, "/tenants/"+t+"/vm/"+action, nil, nil)
}

// HardenedProject mirrors hostd's report for one inner pj-<uuid> project.
type HardenedProject struct {
	Project        string   `json:"project"`
	Updated        []string `json:"updated"`
	PendingRestart []string `json:"pending_restart"`
	Restricted     bool     `json:"restricted"`
	Error          string   `json:"error,omitempty"`
}

// HardenInner applies the pj- restriction set to a tenant VM's existing inner projects.
func (c *Client) HardenInner(ctx context.Context, t, plan string, restart bool) ([]HardenedProject, error) {
	var out struct {
		Projects []HardenedProject `json:"projects"`
	}
	err := c.do(ctx, http.MethodPost, "/tenants/"+t+"/inner-projects/harden",
		map[string]any{"plan": plan, "restart": restart}, &out)
	return out.Projects, err
}
