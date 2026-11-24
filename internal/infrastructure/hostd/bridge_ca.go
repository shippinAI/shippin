package hostd

import (
	"context"
	"fmt"
	"os"
	"regexp"
)

// BridgeCAConfig makes every new workspace's sshd trust the Shippin bridge CA, with the
// workspace's own principal as the only accepted one (N4 devspace access). Empty = off.
type BridgeCAConfig struct {
	PublicKeyFile   string `json:"public_key_file"`  // ssh-ed25519 … line of the CA (public, no secret)
	SSHUser         string `json:"ssh_user"`         // workspace login user, e.g. dev
	PrincipalPrefix string `json:"principal_prefix"` // principal = prefix + workspace uuid
}

var safeName = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// bridgeTrustScript writes the CA (from stdin), principals and the sshd drop-in, then reloads.
const bridgeTrustScript = `set -e
cat > /etc/ssh/shippin_user_ca.pub; chmod 644 /etc/ssh/shippin_user_ca.pub
mkdir -p /etc/ssh/auth_principals; printf '%%s\n' "$1" > "/etc/ssh/auth_principals/$2"; chmod 644 "/etc/ssh/auth_principals/$2"
printf 'TrustedUserCAKeys /etc/ssh/shippin_user_ca.pub\nAuthorizedPrincipalsFile /etc/ssh/auth_principals/%%%%u\n' > /etc/ssh/sshd_config.d/shippin-bridge.conf
sshd -t; systemctl reload ssh || systemctl reload sshd`

// trustBridgeCA installs the bridge CA trust in a workspace (no-op when not configured).
func (s *Server) trustBridgeCA(ctx context.Context, at place, inst, w string) error {
	c := s.cfg.BridgeCA
	if c.PublicKeyFile == "" {
		return nil
	}
	pub, err := os.ReadFile(c.PublicKeyFile)
	if err != nil {
		return fmt.Errorf("bridge ca: %w", err)
	}
	if !safeName.MatchString(c.SSHUser) || c.PrincipalPrefix == "" {
		return fmt.Errorf("bridge ca: ssh_user and principal_prefix are required")
	}
	script := fmt.Sprintf(bridgeTrustScript)
	_, err = at.inc(ctx, string(pub), "exec", inst, "--", "sh", "-c", script, "bridge-ca", c.PrincipalPrefix+w, c.SSHUser)
	return err
}
