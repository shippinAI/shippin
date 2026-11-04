package catalog

import (
	"encoding/json"
	"strings"
	"testing"
)

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

func TestDeployableServiceDefaultsRedactVaultixAndSOPSReferences(t *testing.T) {
	got := RedactSecretReferences([]byte(`{"port":5432,"auth":{"password":"vaultix://tenant/project/key"},"keys":["sops://runtime/secrets.yaml#/token"]}`))
	text := string(got)
	for _, secret := range []string{"vaultix://", "sops://"} {
		if strings.Contains(text, secret) {
			t.Fatalf("secret reference leaked: %s", text)
		}
	}
	var value map[string]any
	if err := json.Unmarshal(got, &value); err != nil {
		t.Fatal(err)
	}
	if value["port"] != float64(5432) {
		t.Fatalf("non-secret default was removed: %s", got)
	}
	authConfig, ok := value["auth"].(map[string]any)
	if !ok || len(authConfig) != 0 {
		t.Fatalf("nested reference wasn't redacted: %s", got)
	}
	keys, ok := value["keys"].([]any)
	if !ok || len(keys) != 1 || keys[0] != "" {
		t.Fatalf("array reference wasn't redacted: %s", got)
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
		"SOPS reference without key pointer": {Kind: "git", Reference: "https://git.example.net/acme/app", Commit: "0123456789abcdef0123456789abcdef01234567", CredentialRef: "sops://runtime/secrets.yaml"},
		"Vaultix reference with fragment":    {Kind: "git", Reference: "https://git.example.net/acme/app", Commit: "0123456789abcdef0123456789abcdef01234567", CredentialRef: "vaultix://org/project/key#/unexpected"},
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
