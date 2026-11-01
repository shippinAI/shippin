package hostd

import (
	"context"
	"fmt"
	"net/url"
	"sync"
)

// Serialize this hostd's attachment operations. Incus also rejects a conflicting device add.
var aiMountMu sync.Mutex

// verifyAIHome reads Incus metadata only, never credentials. Existing legacy workspace
// attachments must be removed through an approved migration before a runner can start.
func (s *Server) verifyAIHome(ctx context.Context, at place, inst, account string) (bool, error) {
	var volume struct {
		Config map[string]string `json:"config"`
		UsedBy []string          `json:"used_by"`
	}
	path := "/1.0/storage-pools/" + at.pool() + "/volumes/custom/" + AIHomeVolume(account)
	if err := at.query(ctx, path, &volume); err != nil {
		return false, fmt.Errorf("AI home metadata unavailable")
	}
	if volume.Config["user.shippin.account"] != account {
		return false, fmt.Errorf("AI home ownership mismatch")
	}
	for _, ref := range volume.UsedBy {
		u, err := url.Parse(ref)
		if err != nil || u.IsAbs() || u.Host != "" || u.Path != "/1.0/instances/"+inst || u.Query().Get("project") != at.project() {
			return false, fmt.Errorf("AI home still attached outside its isolated owner runner")
		}
	}
	var instance struct {
		Config  map[string]string            `json:"config"`
		Devices map[string]map[string]string `json:"expanded_devices"`
	}
	if err := at.query(ctx, "/1.0/instances/"+inst, &instance); err != nil {
		return false, fmt.Errorf("runner metadata unavailable")
	}
	if instance.Config["user.shippin.utility"] != account || instance.Config["user.shippin.tenant"] != at.t {
		return false, fmt.Errorf("runner ownership mismatch")
	}
	if owner := instance.Config["user.shippin.ai_home"]; owner != "" && owner != account {
		return false, fmt.Errorf("runner AI home ownership mismatch")
	}
	d, present := instance.Devices[aiDevice]
	if present && (d["type"] != "disk" || d["pool"] != at.pool() || d["source"] != AIHomeVolume(account) || d["path"] != s.cfg.AI.HomePath) {
		return false, fmt.Errorf("runner AI home device mismatch")
	}
	return present, nil
}
