package catalog

import (
	"encoding/json"
	"testing"
)

const postgresConfigSchema = `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"port":{"type":"integer","minimum":1,"maximum":65535},"password":{"type":"string","format":"shippin-secret-ref"}},"required":["password"],"additionalProperties":false}`

func TestDeployableConfigSchemaValidatesClosedValuesAndSecretReferences(t *testing.T) {
	schema := json.RawMessage(postgresConfigSchema)
	defaults := json.RawMessage(`{"port":5432,"password":"vaultix://production/postgres/password"}`)
	if err := ValidateDeploymentConfig(schema, defaults); err != nil {
		t.Fatalf("valid definition rejected: %v", err)
	}
	if err := ValidateDeploymentConfigValues(schema, json.RawMessage(`{"port":5433,"password":"sops://runtime/secrets.yaml#/postgres/password"}`)); err != nil {
		t.Fatalf("valid instance configuration rejected: %v", err)
	}
	for name, values := range map[string]string{
		"plaintext secret": `{"port":5432,"password":"plaintext-password"}`,
		"extra field":      `{"port":5432,"password":"vaultix://production/postgres/password","admin":true}`,
		"wrong type":       `{"port":"5432","password":"vaultix://production/postgres/password"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateDeploymentConfigValues(schema, json.RawMessage(values)); err == nil {
				t.Fatalf("unsafe config accepted: %s", name)
			}
		})
	}
}

func TestDeployableConfigSchemaRejectsUnsafeOrUnboundedDefinitions(t *testing.T) {
	defaults := json.RawMessage(`{"password":"vaultix://production/postgres/password"}`)
	for name, schema := range map[string]string{
		"open additional properties":                 `{"type":"object","properties":{"password":{"type":"string","format":"shippin-secret-ref"}}}`,
		"object union remains closed":                `{"type":["object","null"],"properties":{"port":{"type":"integer"}}}`,
		"properties imply closed object behavior":    `{"properties":{"port":{"type":"integer"}}}`,
		"unmarked password":                          `{"type":"object","properties":{"password":{"type":"string"}},"additionalProperties":false}`,
		"secret literal default":                     `{"type":"object","properties":{"password":{"type":"string","format":"shippin-secret-ref","default":"vaultix://production/postgres/password"}},"additionalProperties":false}`,
		"remote reference":                           `{"type":"object","properties":{"password":{"$ref":"https://example.invalid/password.json"}},"additionalProperties":false}`,
		"local reference cannot hide a secret field": `{"type":"object","$defs":{"password":{"type":"string"}},"properties":{"password":{"$ref":"#/$defs/password"}},"additionalProperties":false}`,
		"named secret definition must be marked":     `{"type":"object","$defs":{"password":{"type":"string"}},"properties":{"auth":{"$ref":"#/$defs/password"}},"additionalProperties":false}`,
		"dynamic property names":                     `{"type":"object","patternProperties":{".*":{"type":"string"}},"additionalProperties":false}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateDeploymentConfig(json.RawMessage(schema), defaults); err == nil {
				t.Fatalf("unsafe schema accepted: %s", name)
			}
		})
	}
}

func TestSecretReferenceFormatRejectsCredentialsAndFragments(t *testing.T) {
	for _, value := range []string{
		"vaultix://user:password@platform/db",
		"vaultix:plaintext",
		"vaultix://platform/db#unexpected",
		"sops://runtime/secrets.yaml#fragment",
		"sops://runtime/secrets.yaml",
		"https://secrets.example/db",
		"vaultix://platform/db?token=value",
	} {
		if err := validateSecretReferenceValue(value); err == nil {
			t.Errorf("accepted unsafe reference %q", value)
		}
	}
	for _, value := range []string{"vaultix://production/database/postgres/password", "sops://runtime/secrets.yaml#/registry/token"} {
		if err := validateSecretReferenceValue(value); err != nil {
			t.Errorf("rejected valid reference %q: %v", value, err)
		}
	}
}

func TestParseVaultixReferenceUsesTenantProjectAndSplitsEnvironmentPathAndKey(t *testing.T) {
	got, err := ParseVaultixReference("vaultix://production/services/postgres/admin-password")
	if err != nil {
		t.Fatal(err)
	}
	if got.Environment != "production" || got.Path != "/services/postgres" || got.Key != "admin-password" {
		t.Fatalf("unexpected parsed tenant-scoped reference: %+v", got)
	}
	root, err := ParseVaultixReference("vaultix://development/password")
	if err != nil || root.Environment != "development" || root.Path != "/" || root.Key != "password" {
		t.Fatalf("root secret reference: %+v err=%v", root, err)
	}
}

func TestParseVaultixReferenceRejectsProjectSelectorsAndUnsafePaths(t *testing.T) {
	for _, value := range []string{
		"vaultix://user:pass@production/db/password",
		"vaultix://production:443/db/password",
		"vaultix://production/db/../password",
		"vaultix://production//db/password",
		"vaultix://production/db\\password",
		"vaultix://production/db/password?version=latest",
		"vaultix://production/db/password#fragment",
		"vaultix://UPPER/db/password",
		"vaultix://production/",
	} {
		if _, err := ParseVaultixReference(value); err == nil {
			t.Errorf("accepted unsafe tenant secret reference %q", value)
		}
	}
}
