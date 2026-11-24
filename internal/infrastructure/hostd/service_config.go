package hostd

import (
	"fmt"
	"strings"
)

// ServicesConfig: managed service instances si-<uuid> placed in a project's inner Incus project on
// the tenant's boundary VM (07-multi-tenancy "service instance"). A nil kind is disabled.
type ServicesConfig struct {
	RRFlow *RRFlowService `json:"rrflow_state"`
}

// RRFlowService runs rrflow/state (02c-rrflow.md). Binary is the host path of the rrflow-state
// build shipped with this hostd release; it is pushed into the instance and into bound workspaces.
type RRFlowService struct {
	Image   string `json:"image"`
	Binary  string `json:"binary"`
	Port    int    `json:"port"`
	CPU     int    `json:"cpu"`
	Memory  string `json:"memory"`
	DataDir string `json:"data_dir"`
	// WSUser/WSUID own the binding token file in bound workspaces (the image's dev user).
	WSUser string `json:"ws_user"`
	WSUID  int    `json:"ws_uid"`
	// Migrate gates the pre-D36 -> shared move (change ticket B); both default off.
	Migrate RRFlowMigrate `json:"migrate"`
	// AllowDelete enables DELETE .../services/{si} (default off: no instance is ever deleted through
	// the API until a ticket turns it on; retire-into has its own retire_old gate).
	AllowDelete bool `json:"allow_delete"`
}

// RRFlowMigrate: Enabled allows migrate-to (snapshot + copy, never delete); RetireOld allows the
// separate retire call that deletes a source only after the target namespace verifies.
type RRFlowMigrate struct {
	Enabled   bool `json:"enabled"`
	RetireOld bool `json:"retire_old"`
}

func (c ServicesConfig) check(imagePrefix string) error {
	r := c.RRFlow
	if r == nil {
		return nil
	}
	switch {
	case !strings.HasPrefix(r.Image, imagePrefix) || !imageRe.MatchString(r.Image):
		return fmt.Errorf("services.rrflow_state.image must be under edge.image_prefix")
	case !strings.HasPrefix(r.Binary, "/") || strings.Contains(r.Binary, ".."):
		return fmt.Errorf("services.rrflow_state.binary must be an absolute host path")
	case r.Port < 1024 || r.Port > 65535 || r.CPU < 1 || r.Memory == "":
		return fmt.Errorf("services.rrflow_state needs port, cpu and memory")
	case !strings.HasPrefix(r.DataDir, "/var/lib/") || strings.Contains(r.DataDir, ".."):
		return fmt.Errorf("services.rrflow_state.data_dir must be under /var/lib")
	case r.WSUser == "" || r.WSUID < 1:
		return fmt.Errorf("services.rrflow_state needs ws_user and ws_uid")
	}
	return nil
}

// ServiceInstance is the Incus name of a managed service instance.
func ServiceInstance(si string) string { return "si-" + si }
