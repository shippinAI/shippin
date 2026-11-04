package catalog

import (
	"encoding/json"
	"testing"
)

const postgresConfigSchema = `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"port":{"type":"integer","minimum":1,"maximum":65535},"password":{"type":"string","format":"shippin-secret-ref"}},"required":["password"],"additionalProperties":false}`

func TestDeployableConfigSchemaValidatesClosedValuesAndSecretReferences(t *testing.T) {
	schema := json.RawMessage(postgresConfigSchema)
	defaults := json.RawMessage(`{"port":5432,"password":"vaultix://platform/postgres/password"}`)
	if err := ValidateDeploymentConfig(schema, defaults); err != nil {
		t.Fatalf("valid definition rejected: %v", err)
	}
	if err := ValidateDeploymentConfigValues(schema, json.RawMessage(`{"port":5433,"password":"sops://services/postgres/password"}`)); err != nil {
		t.Fatalf("valid instance configuration rejected: %v", err)
	}
	for name, values := range map[string]string{
		"plaintext secret": `{"port":5432,"password":"plaintext-password"}`,
		"extra field":      `{"port":5432,"password":"vaultix://platform/postgres/password","admin":true}`,
		"wrong type":       `{"port":"5432","password":"vaultix://platform/postgres/password"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateDeploymentConfigValues(schema, json.RawMessage(values)); err == nil {
				t.Fatalf("unsafe config accepted: %s", name)
			}
		})
	}
}

func TestDeployableConfigSchemaRejectsUnsafeOrUnboundedDefinitions(t *testing.T) {
	defaults := json.RawMessage(`{"password":"vaultix://platform/postgres/password"}`)
	for name, schema := range map[string]string{
		"open additional properties":                 `{"type":"object","properties":{"password":{"type":"string","format":"shippin-secret-ref"}}}`,
		"object union remains closed":                `{"type":["object","null"],"properties":{"port":{"type":"integer"}}}`,
		"properties imply closed object behavior":    `{"properties":{"port":{"type":"integer"}}}`,
		"unmarked password":                          `{"type":"object","properties":{"password":{"type":"string"}},"additionalProperties":false}`,
		"secret literal default":                     `{"type":"object","properties":{"password":{"type":"string","format":"shippin-secret-ref","default":"vaultix://platform/postgres/password"}},"additionalProperties":false}`,
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
		"sops://platform/db#fragment",
		"https://secrets.example/db",
		"vaultix://platform/db?token=value",
	} {
		if err := validateSecretReferenceValue(value); err == nil {
			t.Errorf("accepted unsafe reference %q", value)
		}
	}
}
