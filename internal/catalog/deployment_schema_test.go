package catalog

import (
	"encoding/json"
	"strings"
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

func TestDeployableConfigAllowsCredentialObjectWithMarkedSecretChild(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"credentials":{"type":"object","properties":{"username":{"type":"string"},"password":{"type":"string","format":"shippin-secret-ref"}},"required":["username","password"],"additionalProperties":false}},"required":["credentials"],"additionalProperties":false}`)
	defaults := json.RawMessage(`{"credentials":{"username":"app","password":"vaultix://production/database/password"}}`)
	if err := ValidateDeploymentConfig(schema, defaults); err != nil {
		t.Fatalf("credential container with declared secret child was rejected: %v", err)
	}
	unsafeSchema := json.RawMessage(strings.Replace(string(schema), `,"format":"shippin-secret-ref"`, "", 1))
	if err := ValidateDeploymentConfig(unsafeSchema, defaults); err == nil {
		t.Fatal("unmarked nested password field was accepted")
	}
}

func TestRuntimeV3EnvironmentBindingsMatchConfigSchemaAndSecretPolicy(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"credentials":{"type":"object","properties":{"password":{"type":"string","format":"shippin-secret-ref"}},"required":["password"],"additionalProperties":false},"database":{"type":"string"},"port":{"type":"integer"}},"required":["credentials","database","port"],"additionalProperties":false}`)
	environment := func(bindings ...RuntimeEnvironmentBinding) *[]RuntimeEnvironmentBinding { return &bindings }
	runtime := RuntimeContract{Version: 3, Environment: environment(
		RuntimeEnvironmentBinding{Name: "DB_PASSWORD", ConfigPath: "/credentials/password"},
		RuntimeEnvironmentBinding{Name: "POSTGRES_DB", ConfigPath: "/database"},
	)}
	values := json.RawMessage(`{"credentials":{"password":"vaultix://production/postgres/password"},"database":"app","port":5432}`)
	if err := ValidateRuntimeConfigBindings(runtime, schema, values); err != nil {
		t.Fatalf("valid nested bindings rejected: %v", err)
	}
	for name, candidate := range map[string]struct {
		runtime RuntimeContract
		values  json.RawMessage
	}{
		"missing schema path":       {runtime: RuntimeContract{Version: 3, Environment: environment(RuntimeEnvironmentBinding{Name: "MISSING", ConfigPath: "/not-declared"})}, values: values},
		"non string config field":   {runtime: RuntimeContract{Version: 3, Environment: environment(RuntimeEnvironmentBinding{Name: "PORT", ConfigPath: "/port"})}, values: values},
		"wrong value type":          {runtime: RuntimeContract{Version: 3, Environment: environment(RuntimeEnvironmentBinding{Name: "DB", ConfigPath: "/database"})}, values: json.RawMessage(`{"credentials":{"password":"vaultix://production/postgres/password"},"database":4,"port":5432}`)},
		"unmarked secret reference": {runtime: RuntimeContract{Version: 3, Environment: environment(RuntimeEnvironmentBinding{Name: "DB", ConfigPath: "/database"})}, values: json.RawMessage(`{"credentials":{"password":"vaultix://production/postgres/password"},"database":"vaultix://production/db/name","port":5432}`)},
		"unsupported SOPS secret":   {runtime: RuntimeContract{Version: 3, Environment: environment(RuntimeEnvironmentBinding{Name: "DB_PASSWORD", ConfigPath: "/credentials/password"})}, values: json.RawMessage(`{"credentials":{"password":"sops://runtime/secrets.yaml#/password"},"database":"app","port":5432}`)},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateRuntimeConfigBindings(candidate.runtime, schema, candidate.values); err == nil {
				t.Fatal("invalid runtime configuration binding accepted")
			}
		})
	}
}

func TestExtractRuntimeEnvironmentInputsKeepsSecretsAsReferences(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"credentials":{"type":"object","properties":{"password":{"type":"string","format":"shippin-secret-ref"}},"required":["password"],"additionalProperties":false},"database":{"type":"string"},"options":{"type":"object","properties":{"quoted/name":{"type":"string"}},"additionalProperties":false}},"required":["credentials","database"],"additionalProperties":false}`)
	environment := []RuntimeEnvironmentBinding{
		{Name: "DB_PASSWORD", ConfigPath: "/credentials/password"},
		{Name: "POSTGRES_DB", ConfigPath: "/database"},
		{Name: "OPTIONAL_VALUE", ConfigPath: "/options/quoted~1name"},
	}
	runtime := RuntimeContract{Version: 3, Environment: &environment}
	values := json.RawMessage(`{"credentials":{"password":"vaultix://production/services/postgres/password"},"database":"app"}`)
	if err := ValidateDeploymentConfigValues(schema, values); err != nil {
		t.Fatalf("valid config rejected before extraction: %v", err)
	}
	inputs, err := ExtractRuntimeEnvironmentInputs(runtime, schema, values)
	if err != nil {
		t.Fatalf("valid runtime inputs rejected: %v", err)
	}
	if len(inputs) != 2 || inputs[0].Name != "DB_PASSWORD" || inputs[0].Value != nil ||
		inputs[0].VaultixReference != "vaultix://production/services/postgres/password" ||
		inputs[1].Name != "POSTGRES_DB" || inputs[1].Value == nil || *inputs[1].Value != "app" || inputs[1].VaultixReference != "" {
		t.Fatalf("unexpected extracted inputs: %+v", inputs)
	}
	withOptional := json.RawMessage(`{"credentials":{"password":"vaultix://production/services/postgres/password"},"database":"app","options":{"quoted/name":"present"}}`)
	inputs, err = ExtractRuntimeEnvironmentInputs(runtime, schema, withOptional)
	if err != nil || len(inputs) != 3 || inputs[2].Name != "OPTIONAL_VALUE" || inputs[2].Value == nil || *inputs[2].Value != "present" {
		t.Fatalf("escaped optional pointer inputs=%+v err=%v", inputs, err)
	}
	emptyLiteral := json.RawMessage(`{"credentials":{"password":"vaultix://production/services/postgres/password"},"database":""}`)
	inputs, err = ExtractRuntimeEnvironmentInputs(runtime, schema, emptyLiteral)
	if err != nil || len(inputs) != 2 || inputs[1].Value == nil || *inputs[1].Value != "" {
		t.Fatalf("empty literal must remain distinct from an unresolved secret: inputs=%+v err=%v", inputs, err)
	}
}

func TestExtractRuntimeEnvironmentInputsFailsClosedWithoutPartialValues(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"database":{"type":"string"},"password":{"type":"string","format":"shippin-secret-ref"}},"required":["database","password"],"additionalProperties":false}`)
	environment := []RuntimeEnvironmentBinding{{Name: "DB", ConfigPath: "/database"}, {Name: "PASSWORD", ConfigPath: "/password"}}
	runtime := RuntimeContract{Version: 3, Environment: &environment}
	for name, values := range map[string]json.RawMessage{
		"malformed secret reference": json.RawMessage(`{"database":"app","password":"vaultix://production/"}`),
		"wrong literal type":         json.RawMessage(`{"database":4,"password":"vaultix://production/db/password"}`),
		"oversized literal":          json.RawMessage(`{"database":"` + strings.Repeat("x", 8193) + `","password":"vaultix://production/db/password"}`),
	} {
		t.Run(name, func(t *testing.T) {
			inputs, err := ExtractRuntimeEnvironmentInputs(runtime, schema, values)
			if err == nil || inputs != nil {
				t.Fatalf("invalid config must return no partial inputs: inputs=%+v err=%v", inputs, err)
			}
			if strings.Contains(err.Error(), "vaultix://") || strings.Contains(err.Error(), strings.Repeat("x", 32)) {
				t.Fatalf("error leaked config material: %v", err)
			}
		})
	}
	if inputs, err := ExtractRuntimeEnvironmentInputs(RuntimeContract{Version: 2}, schema, json.RawMessage(`{"database":"app","password":"vaultix://production/db/password"}`)); err == nil || inputs != nil {
		t.Fatalf("non-v3 runtime must return no inputs: %+v err=%v", inputs, err)
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
