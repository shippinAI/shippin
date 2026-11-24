package catalog

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
)

// DeploymentSource pins the artifact a managed-service runtime may consume. It contains no
// executable command and no credential value. Runtime adapters must enforce egress policy and
// resolve CredentialRef through the configured secret owner before fetching anything.
type DeploymentSource struct {
	Kind          string `json:"kind"`
	Reference     string `json:"reference"`
	Commit        string `json:"commit,omitempty"`
	ContextPath   string `json:"context_path,omitempty"`
	CredentialRef string `json:"credential_ref,omitempty"`
}

var (
	gitCommit = regexp.MustCompile(`^(?:[a-f0-9]{40}|[a-f0-9]{64})$`)
	ociDigest = regexp.MustCompile(`^[a-zA-Z0-9.-]+(?::[0-9]{1,5})?/[a-zA-Z0-9._/-]+@sha256:[a-f0-9]{64}$`)
)

// Validate rejects floating source refs, URLs containing credentials, and fields that do not
// apply to the selected source kind. It validates configuration shape only; it does not fetch or
// deploy the source.
func (s DeploymentSource) Validate() error {
	if len(s.Reference) == 0 || len(s.Reference) > 2048 {
		return fmt.Errorf("source reference is required and must be at most 2048 characters")
	}
	if s.ContextPath != "" && !validContextPath(s.ContextPath) {
		return fmt.Errorf("source context path must be a relative path without parent segments")
	}
	if s.CredentialRef != "" && !validCredentialRef(s.CredentialRef) {
		return fmt.Errorf("credential_ref must be a Vaultix or sops reference, not a credential value")
	}
	switch s.Kind {
	case "github", "git":
		if err := validateGitReference(s.Kind, s.Reference); err != nil {
			return err
		}
		if !gitCommit.MatchString(s.Commit) {
			return fmt.Errorf("git sources must pin a full hexadecimal commit")
		}
	case "oci":
		if !ociDigest.MatchString(s.Reference) || s.Commit != "" || s.ContextPath != "" {
			return fmt.Errorf("OCI sources must use a fully qualified image pinned by sha256 digest")
		}
	default:
		return fmt.Errorf("source kind must be github, git, or oci")
	}
	return nil
}

func validateGitReference(kind, reference string) error {
	u, err := url.Parse(reference)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("git source must be an HTTPS repository URL without credentials, query, or fragment")
	}
	if u.Port() != "" && (u.Port() == "0" || len(u.Port()) > 5) {
		return fmt.Errorf("git source URL has an invalid port")
	}
	if strings.Contains(u.Path, "\\") || strings.Contains(u.Path, "..") {
		return fmt.Errorf("git source URL has an invalid repository path")
	}
	parts := strings.Split(strings.Trim(strings.TrimSuffix(u.Path, ".git"), "/"), "/")
	if len(parts) < 2 || parts[len(parts)-1] == "" {
		return fmt.Errorf("git source URL must identify a repository")
	}
	if kind == "github" && !strings.EqualFold(u.Hostname(), "github.com") {
		return fmt.Errorf("GitHub sources must use github.com")
	}
	if path.Clean(u.Path) != u.Path {
		return fmt.Errorf("git source URL has a non-canonical repository path")
	}
	return nil
}

func validContextPath(value string) bool {
	if len(value) > 512 || strings.Contains(value, "\\") || strings.HasPrefix(value, "/") {
		return false
	}
	clean := path.Clean(value)
	return clean == value && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func validCredentialRef(value string) bool {
	return validateSecretReferenceValue(value) == nil
}
