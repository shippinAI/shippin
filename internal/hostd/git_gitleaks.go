package hostd

import (
	"fmt"
	"os"
	"regexp"
)

// GitleaksPin (B1 finding 5) is the one gitleaks binary publish may run. Version and sha256 live in
// hostd.json, not in code. It runs ON THE HOST (git_hostscan.go) by absolute HostPath after its
// sha256 is re-verified, never inside the devspace where the tenant has root. Path is kept for
// config compatibility. Empty = publish refuses (fail closed).
type GitleaksPin struct {
	Version  string `json:"version"`
	SHA256   string `json:"sha256"`
	HostPath string `json:"host_path"`
	Path     string `json:"path"`
}

var (
	pinShaRe  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	pinPathRe = regexp.MustCompile(`^/usr/local/lib/shippin/[a-z0-9-]{1,40}$`)
	pinVerRe  = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
)

func (p GitleaksPin) check() error {
	if p == (GitleaksPin{}) {
		return nil
	}
	if !pinVerRe.MatchString(p.Version) || !pinShaRe.MatchString(p.SHA256) || !pinPathRe.MatchString(p.HostPath) || !pinPathRe.MatchString(p.Path) {
		return fmt.Errorf("git.gitleaks needs version x.y.z, a 64-hex sha256 and paths under /usr/local/lib/shippin/")
	}
	return nil
}

// readHostFile is a test seam.
var readHostFile = os.ReadFile
