package hostd

import (
	"context"
	"errors"
	"fmt"
	"time"

	"shippin.cloud/console/internal/subs"
)

// UtilityReq starts a user's activation container: a terminal (ttyd + tmux) where they sign in
// to their own AI subscriptions with each vendor's CLI. host is its private edge hostname.
type UtilityReq struct {
	Plan string `json:"plan"`
	Host string `json:"host"`
}

// utilityPlace: legacy tenants use the tenant project; VM tenants get an inner project per
// account (pj-<account>, restricted like every workspace project) so it never shares one with a
// Shippin project's workspaces.
func (s *Server) utilityPlace(ctx context.Context, t, account string, plan Plan) (place, error) {
	if err := ValidUUID(account); err != nil {
		return place{}, fmt.Errorf("invalid account id")
	}
	at, err := s.newPlace(ctx, t)
	if err != nil || !at.vm {
		return at, err
	}
	at = at.in(account)
	return at, s.ensureInnerProject(ctx, at, plan)
}

// StartUtility ensures u-<account> exists with the AI home mounted, runs ttyd, publishes the
// private route and returns the container address.
func (s *Server) StartUtility(ctx context.Context, t, account string, r UtilityReq) (ip string, err error) {
	if ValidUUID(t) != nil || ValidUUID(account) != nil {
		return "", fmt.Errorf("invalid utility identity")
	}
	unlock := lockWorkspace(utilityRouteID(t, account))
	defer unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	a := s.cfg.AI
	if !a.enabled() {
		return "", fmt.Errorf("ai activation is not configured")
	}
	plan, ok := s.cfg.Plans[r.Plan]
	if !ok {
		return "", fmt.Errorf("unknown plan")
	}
	if err := s.ValidHost(r.Host); err != nil {
		return "", err
	}
	if r.Host != subs.TerminalLabel(t, account)+"."+s.cfg.Edge.BaseDomain {
		return "", fmt.Errorf("terminal hostname does not match tenant and account")
	}
	var at place
	owned := false
	defer func() {
		if err != nil {
			if owned {
				err = errors.Join(err, s.cleanupUtility(ctx, at, account))
			} else {
				err = errors.Join(err, s.cleanupUtilityRoute(ctx, t, account))
			}
			ip = ""
		}
	}()
	// Withdraw the deterministic route before discovery, provisioning or mounting.
	if err := s.removeUtilityRoute(ctx, t, account); err != nil {
		return "", err
	}
	at, err = s.utilityPlace(ctx, t, account, plan)
	if err != nil {
		return "", err
	}
	inst := UtilityInstance(account)
	exists, err := utilityExists(ctx, at, account)
	if err != nil {
		return "", err
	}
	if !exists {
		if at.vm {
			if err := s.ensureInnerImage(ctx, at, a.Utility.Image); err != nil {
				return "", err
			}
		}
		args := append([]string{"init", a.Utility.Image, inst, "-c", "user.shippin.tenant=" + t,
			"-c", "user.shippin.utility=" + account}, innerInitArgs(at, plan, Size{})...)
		args = append(args, "-c", fmt.Sprintf("limits.cpu=%d", a.Utility.CPU), "-c", "limits.memory="+a.Utility.Memory)
		if _, err := at.do(ctx, args...); err != nil {
			return "", err
		}
		if exists, err = utilityExists(ctx, at, account); err != nil || !exists {
			return "", errors.Join(fmt.Errorf("created utility ownership unconfirmed"), err)
		}
	}
	owned = true
	if err := s.mountAIHome(ctx, at, inst, account); err != nil {
		return "", err
	}
	if _, err := at.do(ctx, "start", inst); err != nil && !isRunning(err) {
		return "", err
	}
	ip, err = at.waitIP(ctx, inst)
	if err != nil {
		return "", err
	}
	if _, err := at.do(ctx, "exec", inst, "--", "/usr/local/sbin/shippin-ws-init"); err != nil {
		return "", err
	}
	if err := s.ownAIHome(ctx, at, inst, account); err != nil {
		return "", err
	}
	env := fmt.Sprintf("TTYD_PORT=%d\nTTYD_IFACE=%s\n", a.Utility.Port, a.Utility.Iface)
	if _, err := at.inc(ctx, env, "exec", inst, "--", "tee", a.Utility.EnvFile); err != nil {
		return "", err
	}
	if _, err := at.do(ctx, "exec", inst, "--", "systemctl", "enable", "--now", a.Utility.Unit); err != nil {
		return "", err
	}
	if _, err := at.do(ctx, "exec", inst, "--", "systemctl", "restart", a.Utility.Unit); err != nil {
		return "", err
	}
	route := s.proxyRoute(utilityRouteID(t, account), []string{r.Host}, fmt.Sprintf("%s:%d", ip, a.Utility.Port), true, false)
	_, err = s.caddy(ctx, "PUT", fmt.Sprintf("/config/apps/http/servers/%s/routes/0", s.cfg.Edge.Server), route)
	return ip, err
}

// StopUtility unpublishes the activation terminal and stops the container (kept for reuse; the
// logins live on the AI home, not in it).
func (s *Server) StopUtility(ctx context.Context, t, account string) error {
	if ValidUUID(t) != nil || ValidUUID(account) != nil {
		return fmt.Errorf("invalid utility identity")
	}
	unlock := lockWorkspace(utilityRouteID(t, account))
	defer unlock()
	routeErr := s.cleanupUtilityRoute(ctx, t, account)
	lookupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	at, err := s.utilityAt(lookupCtx, t, account)
	if err != nil {
		return errors.Join(routeErr, err)
	}
	exists, err := utilityExists(lookupCtx, at, account)
	if err != nil {
		return errors.Join(routeErr, err)
	}
	if !exists {
		return routeErr
	}
	return errors.Join(routeErr, stopUtilityInstance(ctx, at, account))
}

func utilityRouteID(tenant, account string) string { return "u-" + tenant + "-" + account }

// utilityAt locates an existing activation container without creating anything.
func (s *Server) utilityAt(ctx context.Context, t, account string) (place, error) {
	if err := ValidUUID(account); err != nil {
		return place{}, fmt.Errorf("invalid account id")
	}
	at, err := s.newPlace(ctx, t)
	if err != nil || !at.vm {
		return at, err
	}
	return at.in(account), nil
}
