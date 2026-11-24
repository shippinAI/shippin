package hostd

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// workspaceLabels are the user.shippin.* keys of a new workspace instance. Project,
// environment and boundary make Incus inventory reconcilable against the console's rows;
// any of them that is not a UUID is dropped rather than written.
func workspaceLabels(t, w string, r WorkspaceReq) []string {
	a := []string{"-c", "user.shippin.tenant=" + t, "-c", "user.shippin.workspace=" + w,
		"-c", "user.shippin.name=" + r.Name, "-c", "user.shippin.created_by=" + r.Creator}
	for _, kv := range [][2]string{{"project", r.Project}, {"environment", r.Environment}, {"boundary", r.Boundary}} {
		if kv[1] != "" && ValidUUID(kv[1]) == nil {
			a = append(a, "-c", "user.shippin."+kv[0]+"="+kv[1])
		}
	}
	return a
}

func isRunning(err error) bool { return err != nil && strings.Contains(err.Error(), "already running") }

// waitIP polls the instance state for its eth0 global IPv4 (up to 60 s), honouring ctx.
func (p place) waitIP(ctx context.Context, inst string) (string, error) {
	for i := 0; i < 60; i++ {
		var st struct {
			Network map[string]struct {
				Addresses []struct{ Family, Address, Scope string } `json:"addresses"`
			} `json:"network"`
		}
		if p.query(ctx, "/1.0/instances/"+inst+"/state", &st) == nil {
			for _, a := range st.Network["eth0"].Addresses {
				if a.Family == "inet" && a.Scope == "global" {
					return a.Address, p.checkIP(a.Address)
				}
			}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return "", fmt.Errorf("workspace did not get an address")
}
