package hostd

import (
	"context"
	"fmt"
)

// dropFirewall removes every ingress rule tagged ws-<w> (user rules and the edge → agent
// terminal rule) when the workspace is deleted, so no allow rule outlives its address.
func (s *Server) dropFirewall(ctx context.Context, t, w string) error {
	var acl struct {
		Ingress []aclRule `json:"ingress"`
	}
	if err := query(ctx, fmt.Sprintf("/1.0/network-acls/%s?project=%s", aclName, Project(t)), &acl); err != nil {
		return err
	}
	for _, a := range acl.Ingress {
		if a.Description != "ws-"+w {
			continue
		}
		if _, err := incus(ctx, append([]string{"network", "acl", "rule", "remove", aclName, "ingress", "--project=" + Project(t)}, a.args()...)...); err != nil {
			return err
		}
	}
	return nil
}
