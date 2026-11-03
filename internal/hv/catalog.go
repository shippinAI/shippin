package hv

import "slices"

// Attach field names shared by the API and the form it describes.
const (
	FieldBaseURL  = "base_url"
	FieldNode     = "node"
	FieldTokenID  = "token_id"
	FieldTokenEnv = "token_env"
	FieldCAFile   = "ca_file"
	FieldCertFile = "cert_file"
	FieldKeyFile  = "key_file"
)

// Capabilities a driver may offer beyond reading health and guests.
const (
	CapLifecycle    = "lifecycle"
	CapSnapshot     = "snapshot"
	CapSnapshotList = "snapshot_list"
	CapRollback     = "rollback"
	CapCreate       = "create"
	CapTasks        = "tasks"
)

// Field is one value a driver needs to reach a hypervisor.
type Field struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
}

// Spec states what a driver needs and can do. These are facts about the code, so they live in
// code. What a person reads about a driver comes from configuration (ui.providers).
type Spec struct {
	Kind         string   `json:"kind"`
	Fields       []Field  `json:"fields"`
	Capabilities []string `json:"capabilities"`
	Verbs        []string `json:"verbs"`
}

// Specs lists the drivers that can be attached, in their default order.
func Specs() []Spec {
	return []Spec{
		{
			Kind:         "incus",
			Fields:       []Field{{FieldBaseURL, true}, {FieldNode, false}, {FieldCertFile, false}, {FieldKeyFile, false}, {FieldTokenEnv, false}, {FieldCAFile, false}},
			Capabilities: []string{CapLifecycle, CapSnapshot},
			Verbs:        []string{"start", "stop", "shutdown", "reboot"},
		},
		{
			Kind:         "proxmox",
			Fields:       []Field{{FieldBaseURL, true}, {FieldNode, true}, {FieldTokenID, true}, {FieldTokenEnv, true}, {FieldCAFile, false}},
			Capabilities: []string{CapLifecycle, CapSnapshot, CapSnapshotList, CapRollback, CapCreate, CapTasks},
			Verbs:        []string{"start", "stop", "shutdown", "reboot"},
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
