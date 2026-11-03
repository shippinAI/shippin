package hostd

import (
	"encoding/json"
	"os"
	"shippin.cloud/console/internal/enroll"
)

// Plan holds per-tier limits (07b: all values come from plan config).
type Plan struct {
	QuotaGiB     int    `json:"quota_gib"`
	Instances    int    `json:"instances"`
	CPU          int    `json:"cpu"`
	Memory       string `json:"memory"`
	RootGiB      int    `json:"root_gib"`
	HomeGiB      int    `json:"home_gib"`
	WorkspaceCPU int    `json:"workspace_cpu"`
	WorkspaceMem string `json:"workspace_memory"`
	// VM sizes the tenant VM; a plan without it cannot be VM-backed.
	VM *VMPlan `json:"vm"`
	// Boundaries is how many boundary VMs (b-<uuid>) the tier gets; 0 = none (M1: founder = 1).
	Boundaries int `json:"boundaries"`
	// Cowork is a proposal flag: the tier's VM has the RAM and the data-volume room for one
	// Cowork disk (CoworkDiskGiB). The disk is never the VM root. No desktop is installed.
	Cowork bool `json:"cowork"`
}

// VMPlan is the tenant VM size (vm-first tenants, docs/architecture/vm-first-tenants.md).
type VMPlan struct {
	CPU     int    `json:"cpu"`
	Memory  string `json:"memory"`
	RootGiB int    `json:"root_gib"`
	DataGiB int    `json:"data_gib"`
}

type Config struct {
	Socket      string `json:"socket"`
	DatasetRoot string `json:"dataset_root"`
	Uplink      string `json:"uplink"`
	// Transit is the non-LAN uplink for tenant OVN networks (router IPs come from its ovn range).
	Transit     string          `json:"transit"`
	TenantCIDR  string          `json:"tenant_cidr"`
	StateDir    string          `json:"state_dir"`
	Image       string          `json:"image"`
	KeyStore    string          `json:"key_store"`
	PortMin     int             `json:"port_min"`
	PortMax     int             `json:"port_max"`
	Platform    string          `json:"platform_project"`
	HiddenPools []string        `json:"hidden_pools"`
	Plans       map[string]Plan `json:"plans"`
	Edge        Edge            `json:"edge"`
	// VMFirst: new tenants get a tenant VM and their workspaces nest inside it.
	VMFirst bool `json:"vm_first"`
	// TenantVMImage is the tenant VM image alias (deploy/image/build-tenant-vm.sh).
	TenantVMImage string `json:"tenant_vm_image"`
	// TenantVMInit (optional) is a host path to shippin-tvm-init, pushed into the VM before each run
	// so script fixes reach existing VMs without an image rebuild.
	TenantVMInit string `json:"tenant_vm_init"`
	// AI is M2 subscription activation (ai_config.go).
	AI AIConfig `json:"ai"`
	// Git is M3's per-workspace GitHub installation token file (git_token.go).
	Git GitConfig `json:"git"`
	// Connect is "Connect accounts": in-workspace CLI logins and the credential writer (connect_config.go).
	Connect ConnectConfig `json:"connect"`
	// Agent is M4's in-workspace build agent (agent_config.go).
	Agent AgentConfig `json:"agent"`
	// Services are managed service instances on a tenant's boundary VM (service_config.go).
	Services ServicesConfig `json:"services"`
	// Costs price per-project resource use in the control panel (metrics_cost.go).
	Costs CostRates `json:"costs"`
	// BridgeCA: sshd in every new workspace trusts the Shippin bridge CA (bridge_ca.go).
	BridgeCA BridgeCAConfig `json:"bridge_ca"`
	// StarterDB: per-customer starter Postgres at signup (starter_db.go).
	StarterDB StarterDBConfig `json:"starter_db"`
	// Chat: named commands for the console chat (chat_exec.go).
	Chat ChatConfig `json:"chat"`
	// Enroll (CP-P1, optional): register with the console and heartbeat (internal/enroll).
	Enroll *enroll.Config `json:"enroll,omitempty"`
}

func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := &Config{}
	if err := json.Unmarshal(raw, c); err != nil {
		return nil, err
	}
	return c, c.Check()
}

// Edge describes how workspaces are published through the Caddy edge.
type Edge struct {
	Instance        string `json:"instance"`
	IP              string `json:"ip"`
	PlatformNetwork string `json:"platform_network"`
	Console         string `json:"console_upstream"`
	BaseDomain      string `json:"ws_base_domain"`
	IDEPort         int    `json:"ide_port"`
	Server          string `json:"caddy_server"`
	// Hub: OVN network on the transit uplink that every tenant network peers with; the edge
	// sits on it at HubIP and reaches tenant subnets (TenantCIDR) through the peering.
	HubNetwork string `json:"hub_network"`
	HubIP      string `json:"hub_ip"`
	HubIface   string `json:"hub_iface"`
	// PublicIP is the one shared endpoint (edge forward) for SSH and published ports.
	PublicIP string `json:"public_ip"`
	// PreviewPorts is the dev-server port range the edge may reach (e.g. "1024-65535").
	PreviewPorts string `json:"preview_ports"`
	PreviewMin   int    `json:"preview_min"`
	PreviewMax   int    `json:"preview_max"`
	// ImagePrefix restricts template images (e.g. "shippin/").
	ImagePrefix string `json:"image_prefix"`
	// CaddyAdmin is the edge Caddy admin API base, reached from inside the edge instance.
	CaddyAdmin string `json:"caddy_admin"`
	// WSCookie is the console's workspace-host session cookie; it is stripped before upstreams.
	WSCookie string `json:"ws_cookie"`
	// RepoHosts are the git hosts a template workspace may clone from (public https only).
	RepoHosts []string `json:"repo_hosts"`
	// ReservedPorts can never be published on the shared endpoint (e.g. the edge's 80/443).
	ReservedPorts []int `json:"reserved_ports"`
	// ReservedDomains: custom domains equal to or under these are refused (platform hostnames).
	ReservedDomains []string `json:"reserved_domains"`
}
