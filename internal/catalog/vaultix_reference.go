package catalog

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

var (
	vaultixEnvironment = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)
	vaultixKey         = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
)

// VaultixReference addresses a key inside the authenticated tenant's Vaultix
// project. The URI carries no project or organization identifier: those are
// resolved from the Shippin tenant and installation configuration.
type VaultixReference struct {
	Environment string
	Path        string
	Key         string
}

// ParseVaultixReference parses vaultix://<environment>/<folder...>/<key>.
// Callers must derive the project from the authenticated tenant, then check
// Environment against the installation's configured Vaultix environments.
func ParseVaultixReference(value string) (VaultixReference, error) {
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "vaultix" || u.Opaque != "" || u.User != nil ||
		u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" ||
		u.Host == "" || u.Host != u.Hostname() || !vaultixEnvironment.MatchString(u.Host) {
		return VaultixReference{}, errors.New("Vaultix reference must use vaultix://<environment>/<folder...>/<key>")
	}
	if !strings.HasPrefix(u.Path, "/") || strings.Contains(u.Path, "\\") || strings.Contains(u.Path, "//") {
		return VaultixReference{}, errors.New("Vaultix reference path is invalid")
	}
	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) == 0 || !vaultixKey.MatchString(parts[len(parts)-1]) {
		return VaultixReference{}, errors.New("Vaultix reference must end with a valid key name")
	}
	for _, part := range parts[:len(parts)-1] {
		if !vaultixKey.MatchString(part) {
			return VaultixReference{}, errors.New("Vaultix reference contains an invalid folder name")
		}
	}
	path := "/"
	if len(parts) > 1 {
		path += strings.Join(parts[:len(parts)-1], "/")
	}
	return VaultixReference{Environment: u.Host, Path: path, Key: parts[len(parts)-1]}, nil
}
