package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// configBlock is the ssh_config text for one workspace. Paths are absolute and quoted: ssh expands
// `~` from the account database rather than $HOME, and Windows home folders can hold spaces.
func configBlock(w workspace, user, bridge, dir string) string {
	alias := "shippin-" + w.Name
	path := func(name string) string { return `"` + filepath.ToSlash(filepath.Join(dir, name)) + `"` }
	return fmt.Sprintf(`# >>> shippin-bridge %[1]s (%[2]s): ssh %[3]s, or host %[3]s in Claude Desktop
Match host %[3]s exec "%[4]s cert %[2]s"
  HostName %[2]s
  User %[5]s
  ProxyCommand %[4]s proxy %[2]s
  IdentityFile %[6]s
  CertificateFile %[7]s
  IdentitiesOnly yes
  HostKeyAlias %[3]s
  UserKnownHostsFile %[8]s
  StrictHostKeyChecking accept-new
# <<< shippin-bridge %[1]s
`, w.Name, w.ID, alias, bridge, user,
		path(filepath.Join("ssh", w.ID)), path(filepath.Join("ssh", w.ID+"-cert.pub")), path("known_hosts"))
}

// bridgeCommand is how ssh should start this program: by name when it is on PATH, otherwise by
// its absolute path with forward slashes, which ssh's config parser leaves intact on Windows too.
// A path with spaces would need quoting that ssh_config and ProxyCommand read differently, so it
// is refused with the fix instead.
func bridgeCommand() (string, error) {
	self, err := os.Executable()
	if err != nil {
		return "", err
	}
	if found, err := exec.LookPath("shippin-bridge"); err == nil && sameFile(found, self) {
		return "shippin-bridge", nil
	}
	if strings.ContainsAny(self, " \t\"'") {
		return "", fmt.Errorf("%s has spaces or quotes in its path; put shippin-bridge on your PATH (or move it), then run ssh-config again", self)
	}
	return filepath.ToSlash(self), nil
}

func sameFile(a, b string) bool {
	sa, errA := os.Stat(a)
	sb, errB := os.Stat(b)
	return errA == nil && errB == nil && os.SameFile(sa, sb)
}
