package catalog

import "testing"

func TestDeploymentSourceValidation(t *testing.T) {
	valid := []DeploymentSource{
		{Kind: "github", Reference: "https://github.com/acme/postgres.git", Commit: "0123456789abcdef0123456789abcdef01234567", ContextPath: "deploy/postgres", CredentialRef: "vaultix://org/project/runtime/git-token"},
		{Kind: "git", Reference: "https://git.example.net/platform/cache", Commit: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", CredentialRef: "sops://runtime/secrets.yaml#/registry/token"},
		{Kind: "oci", Reference: "registry.example.net/data/postgres@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
	}
	for _, source := range valid {
		if err := source.Validate(); err != nil {
			t.Errorf("valid source %+v rejected: %v", source, err)
		}
	}
}

func TestDeploymentSourceRejectsFloatingOrUnsafeReferences(t *testing.T) {
	invalid := map[string]DeploymentSource{
		"unrecognized kind":                  {Kind: "compose", Reference: "https://github.com/acme/app", Commit: "0123456789abcdef0123456789abcdef01234567"},
		"branch instead of immutable commit": {Kind: "github", Reference: "https://github.com/acme/app", Commit: "main"},
		"non GitHub host":                    {Kind: "github", Reference: "https://git.example.net/acme/app", Commit: "0123456789abcdef0123456789abcdef01234567"},
		"plain HTTP":                         {Kind: "git", Reference: "http://git.example.net/acme/app", Commit: "0123456789abcdef0123456789abcdef01234567"},
		"embedded credential":                {Kind: "git", Reference: "https://user:password@git.example.net/acme/app", Commit: "0123456789abcdef0123456789abcdef01234567"},
		"query string":                       {Kind: "git", Reference: "https://git.example.net/acme/app?token=value", Commit: "0123456789abcdef0123456789abcdef01234567"},
		"parent path":                        {Kind: "git", Reference: "https://git.example.net/acme/app", Commit: "0123456789abcdef0123456789abcdef01234567", ContextPath: "../secret"},
		"credential literal":                 {Kind: "git", Reference: "https://git.example.net/acme/app", Commit: "0123456789abcdef0123456789abcdef01234567", CredentialRef: "password-value"},
		"floating OCI tag":                   {Kind: "oci", Reference: "registry.example.net/data/postgres:latest"},
		"OCI with a git commit":              {Kind: "oci", Reference: "registry.example.net/data/postgres@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Commit: "0123456789abcdef0123456789abcdef01234567"},
	}
	for name, source := range invalid {
		t.Run(name, func(t *testing.T) {
			if err := source.Validate(); err == nil {
				t.Fatalf("unsafe source accepted: %+v", source)
			}
		})
	}
}
