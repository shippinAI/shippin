package hv

import (
	"context"
	"slices"
)

// Attach field names shared by the API and the form it describes.
const (
	FieldBaseURL     = "base_url"
	FieldNode        = "node"
	FieldTokenID     = "token_id"
	FieldTokenSecret = "token_secret"
	FieldTokenEnv    = "token_env"
	FieldTrustToken  = "trust_token"
	FieldCAFile      = "ca_file"
	FieldCertFile    = "cert_file"
	FieldKeyFile     = "key_file"
)

// Capabilities a driver may offer beyond reading health and guests.
const (
	CapLifecycle    = "lifecycle"
	CapSnapshot     = "snapshot"
	CapSnapshotList = "snapshot_list"
	CapRollback     = "rollback"
	CapCreate       = "create"
	CapDelete       = "delete"
	CapTasks        = "tasks"
)

// Field is one value a driver needs to reach a hypervisor. Secret is write-only: the API never
// returns it. Advanced keeps a rarely needed field out of the way in the form.
type Field struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
	Secret   bool   `json:"secret,omitempty"`
	Advanced bool   `json:"advanced,omitempty"`
}

// Spec states what a driver needs and can do. These are facts about the code, so they live in
// code. What a person reads about a driver comes from configuration (ui.providers).
type Spec struct {
	Kind         string   `json:"kind"`
	Fields       []Field  `json:"fields"`
	Capabilities []string `json:"capabilities"`
	Verbs        []string `json:"verbs"`
	// CreateFields are the optional things a new guest can be given on this driver. "blank_vm" says a
	// virtual machine needs no image.
	CreateFields []string `json:"create_fields,omitempty"`
}

// Specs lists the drivers that can be attached, in their default order.
func Specs() []Spec {
	return []Spec{
		{
			Kind: "incus",
			Fields: []Field{
				{Name: FieldTrustToken, Secret: true},
				{Name: FieldNode},
				{Name: FieldBaseURL, Advanced: true},
				{Name: FieldCertFile, Advanced: true},
				{Name: FieldKeyFile, Advanced: true},
				{Name: FieldTokenEnv, Advanced: true},
				{Name: FieldCAFile, Advanced: true},
			},
			Capabilities: []string{CapLifecycle, CapSnapshot, CapSnapshotList, CapRollback, CapCreate, CapDelete},
			Verbs:        []string{"start", "stop", "shutdown", "reboot"},
			CreateFields: []string{"memory", "cores"},
		},
		{
			Kind: "proxmox",
			Fields: []Field{
				{Name: FieldBaseURL, Required: true},
				{Name: FieldTokenID, Required: true},
				{Name: FieldTokenSecret, Secret: true},
				{Name: FieldNode},
				{Name: FieldTokenEnv, Advanced: true},
				{Name: FieldCAFile, Advanced: true},
			},
			Capabilities: []string{CapLifecycle, CapSnapshot, CapSnapshotList, CapRollback, CapCreate, CapDelete, CapTasks},
			Verbs:        []string{"start", "stop", "shutdown", "reboot"},
			CreateFields: []string{"memory", "cores", "disk", "storage", "network", "blank_vm"},
		},
	}
}

// SpecFor returns the spec of kind.
func SpecFor(kind string) (Spec, bool) {
	specs := Specs()
	i := slices.IndexFunc(specs, func(s Spec) bool { return s.Kind == kind })
	if i < 0 {
		return Spec{}, false
	}
	return specs[i], true
}

// ClusterMember is one member of a hypervisor's own cluster, as it reports.
type ClusterMember struct {
	Name   string `json:"name"`
	Online bool   `json:"online"`
	Local  bool   `json:"local,omitempty"`
}

// NodeDetails is what a host says about itself. Anything it did not report stays empty.
type NodeDetails struct {
	Hostname string          `json:"hostname,omitempty"`
	Release  string          `json:"release,omitempty"`
	OS       string          `json:"os,omitempty"`
	Kernel   string          `json:"kernel,omitempty"`
	CPUModel string          `json:"cpu_model,omitempty"`
	CPUs     int             `json:"cpus,omitempty"`
	MemTotal int64           `json:"mem_total_bytes,omitempty"`
	MemUsed  int64           `json:"mem_used_bytes,omitempty"`
	Uptime   int64           `json:"uptime_seconds,omitempty"`
	Cluster  string          `json:"cluster,omitempty"`
	Members  []ClusterMember `json:"members,omitempty"`
}

// Inspector is a driver that can describe its host. A token may lack the right to, so the overview
// treats a failure here as missing detail rather than a host that is down.
type Inspector interface {
	Details(ctx context.Context) (NodeDetails, error)
}

// NodeLister is a driver that addresses work to named nodes and can list them.
type NodeLister interface {
	Nodes(ctx context.Context) ([]string, error)
}
