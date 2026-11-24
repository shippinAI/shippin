package hostd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// caddyRouteStatus discards the response body; only an explicit HTTP status is
// accepted. Transport failures and an empty result are never proof of absence.
func (s *Server) caddyRouteStatus(ctx context.Context, method, id string) (string, error) {
	out, err := runner(ctx, "", "incus", "exec", s.cfg.Edge.Instance, "--project="+s.cfg.Platform, "--",
		"curl", "-sS", "--max-time", "5", "-o", "/dev/null", "-w", "%{http_code}", "-X", method,
		s.cfg.Edge.CaddyAdmin+"/id/"+id)
	return strings.TrimSpace(out), err
}

func (s *Server) removeUtilityRoute(ctx context.Context, tenant, account string) error {
	id := utilityRouteID(tenant, account)
	code, err := s.caddyRouteStatus(ctx, "DELETE", id)
	if err != nil || (code != "200" && code != "404") {
		return errors.Join(fmt.Errorf("utility route deletion unconfirmed (HTTP %q)", code), err)
	}
	code, err = s.caddyRouteStatus(ctx, "GET", id)
	if err != nil || code != "404" {
		return errors.Join(fmt.Errorf("utility route absence unconfirmed (HTTP %q)", code), err)
	}
	return nil
}

// cleanupUtility gives route removal and stop independent time budgets. Always
// attempt both, preserve the instance/volume, and keep failures pending retry.
// Network isolation must still prevent stale routes reaching recycled addresses.
func (s *Server) cleanupUtility(ctx context.Context, at place, account string) error {
	routeErr := s.cleanupUtilityRoute(ctx, at.t, account)
	return errors.Join(routeErr, stopUtilityInstance(ctx, at, account))
}

func stopUtilityInstance(ctx context.Context, at place, account string) error {
	base := context.WithoutCancel(ctx)
	stopCtx, stopCancel := context.WithTimeout(base, 35*time.Second)
	inst := UtilityInstance(account)
	_, stopErr := at.do(stopCtx, "stop", inst, "--timeout=30", "--force")
	stopCancel()
	verifyCtx, verifyCancel := context.WithTimeout(base, 5*time.Second)
	defer verifyCancel()
	var state struct {
		StatusCode int `json:"status_code"`
	}
	stateErr := at.query(verifyCtx, "/1.0/instances/"+inst+"/state", &state)
	if stateErr == nil && state.StatusCode == 102 {
		stopErr = nil // Already stopped is also a verified successful outcome.
	} else {
		stopErr = errors.Join(stopErr, stateErr, fmt.Errorf("utility stopped state unconfirmed"))
	}
	return stopErr
}

func (s *Server) cleanupUtilityRoute(ctx context.Context, tenant, account string) error {
	routeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 12*time.Second)
	defer cancel()
	return s.removeUtilityRoute(routeCtx, tenant, account)
}
