package hostd

import (
	"fmt"
	"regexp"
	"strings"
)

// AIConfig (M2) places each user's AI home volume ai-<account> and their activation container
// u-<account>. Unset (home_path empty) disables every AI route.
type AIConfig struct {
	// HomePath is where the AI home is mounted in workspaces and the activation container.
	HomePath string `json:"home_path"`
	HomeGiB  int    `json:"home_gib"`
	// Shifted sets security.shifted on the volume so one user's isolated-idmap containers all see
	// their own uid on it. Needs idmapped mounts on the pool; verify before enabling.
	Shifted bool `json:"volume_shifted"`
	// User/UID own the AI home inside containers (the image's dev user).
	User string `json:"user"`
	UID  int    `json:"uid"`
	// StatusPath/StatusUnit/StatusMaxBytes: the image's exit-code-only status file and its refresher.
	StatusPath     string    `json:"status_path"`
	StatusUnit     string    `json:"status_unit"`
	StatusMaxBytes int       `json:"status_max_bytes"`
	Utility        AIUtility `json:"utility"`
	// Logout maps a provider id to the vendor's own sign-out argv, run as the user.
	Logout map[string][]string `json:"logout"`
}

// AIUtility is the per-user activation container: ttyd + tmux, private behind forward_auth.
type AIUtility struct {
	Image   string `json:"image"`
	CPU     int    `json:"cpu"`
	Memory  string `json:"memory"`
	Port    int    `json:"ttyd_port"`
	Iface   string `json:"ttyd_iface"`
	Unit    string `json:"ttyd_unit"`
	EnvFile string `json:"ttyd_env_file"`
}

var (
	unitRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}\.service$`)
	ifaceRe = regexp.MustCompile(`^[a-z0-9]{1,15}$`)
	provRe  = regexp.MustCompile(`^[a-z][a-z0-9-]{1,31}$`)
)

func (a AIConfig) enabled() bool { return a.HomePath != "" }

// check fails closed on an enabled but incomplete section.
func (a AIConfig) check(imagePrefix string) error {
	if !a.enabled() {
		return nil
	}
	u := a.Utility
	switch {
	case !strings.HasPrefix(a.HomePath, "/home/") || strings.Contains(a.HomePath, ".."):
		return fmt.Errorf("ai.home_path must be under /home")
	case a.HomeGiB < 1 || a.User == "" || a.UID < 1 || a.StatusMaxBytes < 256 || a.StatusMaxBytes > 1<<16:
		return fmt.Errorf("ai.home_gib, user, uid and status_max_bytes (256-65536) are required")
	case !strings.HasPrefix(a.StatusPath, "/run/") || !unitRe.MatchString(a.StatusUnit):
		return fmt.Errorf("ai.status_path must be under /run and ai.status_unit a .service")
	case u.Image == "" || !strings.HasPrefix(u.Image, imagePrefix) || u.CPU < 1 || u.Memory == "":
		return fmt.Errorf("ai.utility needs an image under edge.image_prefix, cpu and memory")
	case u.Port < 1024 || u.Port > 65535 || !ifaceRe.MatchString(u.Iface) || !unitRe.MatchString(u.Unit):
		return fmt.Errorf("ai.utility needs ttyd_port, ttyd_iface and ttyd_unit")
	case !strings.HasPrefix(u.EnvFile, "/etc/default/"):
		return fmt.Errorf("ai.utility.ttyd_env_file must be under /etc/default")
	}
	for p, argv := range a.Logout {
		if !provRe.MatchString(p) || len(argv) == 0 {
			return fmt.Errorf("ai.logout.%s must be a provider id with an argv", p)
		}
	}
	return nil
}
