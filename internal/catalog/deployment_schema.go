package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	maxDeploymentSchemaBytes = 64 << 10
	maxDeploymentConfigBytes = 64 << 10
	deploymentSchemaURL      = "https://schemas.shippin.invalid/catalog/deployment-config.json"
	secretReferenceFormat    = "shippin-secret-ref"
)

// ValidateDeploymentConfig validates a catalog service's closed parameter schema and defaults.
// It is used only when the service is marked deployable with a DeploymentSource, preserving the
// historical metadata-only catalog entries while refusing unsafe deployment definitions.
func ValidateDeploymentConfig(schemaRaw, defaultsRaw json.RawMessage) error {
	if len(schemaRaw) == 0 || len(schemaRaw) > maxDeploymentSchemaBytes {
		return errors.New("configuration schema is missing or exceeds 64 KiB")
	}
	var schemaDoc map[string]any
	if err := json.Unmarshal(schemaRaw, &schemaDoc); err != nil || schemaDoc == nil {
		return errors.New("configuration schema must be a JSON object")
	}
	if err := validateSchemaPolicy(schemaDoc, true); err != nil {
		return err
	}
	compiled, err := compileDeploymentSchema(schemaDoc)
	if err != nil {
		return errors.New("configuration schema is invalid or references an unavailable schema")
	}
	if len(defaultsRaw) == 0 || len(defaultsRaw) > maxDeploymentConfigBytes {
		return errors.New("default configuration is missing or exceeds 64 KiB")
	}
	defaults, err := decodeJSONValue(defaultsRaw)
	if err != nil {
		return errors.New("default configuration must be valid JSON")
	}
	if err := compiled.Validate(defaults); err != nil {
		return errors.New("default configuration does not match its schema; secret fields must contain secret references")
	}
	return nil
}

// ValidateDeploymentConfigValues applies a previously stored service schema to one deployment's
// non-secret parameters. Secret fields accept only references; resolution belongs to the configured
// secret owner and occurs later in the runtime.
func ValidateDeploymentConfigValues(schemaRaw, valuesRaw json.RawMessage) error {
	if len(valuesRaw) == 0 || len(valuesRaw) > maxDeploymentConfigBytes {
		return errors.New("deployment configuration is missing or exceeds 64 KiB")
	}
	var schemaDoc map[string]any
	if err := json.Unmarshal(schemaRaw, &schemaDoc); err != nil || schemaDoc == nil {
		return errors.New("stored configuration schema is invalid")
	}
	if err := validateSchemaPolicy(schemaDoc, true); err != nil {
		return err
	}
	compiled, err := compileDeploymentSchema(schemaDoc)
	if err != nil {
		return errors.New("stored configuration schema is invalid")
	}
	values, err := decodeJSONValue(valuesRaw)
	if err != nil {
		return errors.New("deployment configuration must be valid JSON")
	}
	if err := compiled.Validate(values); err != nil {
		return errors.New("deployment configuration does not match the service schema; secret fields must contain secret references")
	}
	return nil
}

func compileDeploymentSchema(doc map[string]any) (*jsonschema.Schema, error) {
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	compiler.UseLoader(rejectRemoteSchemaLoader{})
	compiler.RegisterFormat(&jsonschema.Format{Name: secretReferenceFormat, Validate: validateSecretReferenceValue})
	if err := compiler.AddResource(deploymentSchemaURL, doc); err != nil {
		return nil, err
	}
	return compiler.Compile(deploymentSchemaURL)
}

type rejectRemoteSchemaLoader struct{}

func (rejectRemoteSchemaLoader) Load(location string) (any, error) {
	return nil, fmt.Errorf("remote JSON schema references are disabled: %s", location)
}

func decodeJSONValue(raw json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		if err == nil {
			return nil, errors.New("multiple JSON values")
		}
		return nil, err
	}
	return value, nil
}

func validateSchemaPolicy(schema map[string]any, root bool) error {
	if id, ok := schema["$id"]; ok && id != "" {
		return errors.New("$id is not supported in deployment schemas")
	}
	if ref, ok := schema["$ref"].(string); ok && !strings.HasPrefix(ref, "#") {
		return errors.New("remote JSON schema references are disabled")
	}
	for _, unsupported := range []string{"patternProperties", "propertyNames", "dependentSchemas", "unevaluatedProperties"} {
		if _, exists := schema[unsupported]; exists {
			return fmt.Errorf("%s is not supported in deployment schemas", unsupported)
		}
	}
	if dialect, ok := schema["$schema"].(string); ok && dialect != "https://json-schema.org/draft/2020-12/schema" {
		return errors.New("deployment schemas must use JSON Schema 2020-12")
	}
	if schemaMayDescribeObject(schema) {
		if allow, ok := schema["additionalProperties"].(bool); !ok || allow {
			return errors.New("object schemas must set additionalProperties to false")
		}
	}
	if root {
		if schema["type"] != "object" {
			return errors.New("deployment configuration schema root must have type object")
		}
	}
	if schema["format"] == secretReferenceFormat {
		if schema["type"] != "string" {
			return errors.New("secret references must use string fields")
		}
		if _, hasDefault := schema["default"]; hasDefault {
			return errors.New("secret-reference fields cannot declare a default value")
		}
	}
	if properties, ok := schema["properties"].(map[string]any); ok {
		for name, raw := range properties {
			property, ok := raw.(map[string]any)
			if !ok {
				return errors.New("configuration properties must be schema objects")
			}
			if isSensitiveFieldName(name) && property["format"] != secretReferenceFormat {
				return fmt.Errorf("sensitive parameter %q must use format %q", name, secretReferenceFormat)
			}
			if err := validateSchemaPolicy(property, false); err != nil {
				return err
			}
		}
	}
	for _, key := range []string{"$defs", "definitions"} {
		if defs, ok := schema[key].(map[string]any); ok {
			for name, raw := range defs {
				nested, ok := raw.(map[string]any)
				if !ok {
					return errors.New("schema definitions must be objects")
				}
				if isSensitiveFieldName(name) && nested["format"] != secretReferenceFormat {
					return fmt.Errorf("sensitive schema definition %q must use format %q", name, secretReferenceFormat)
				}
				if err := validateSchemaPolicy(nested, false); err != nil {
					return err
				}
			}
		}
	}
	for _, key := range []string{"items", "additionalProperties", "contains", "not", "if", "then", "else", "contentSchema"} {
		if nested, ok := schema[key].(map[string]any); ok {
			if err := validateSchemaPolicy(nested, false); err != nil {
				return err
			}
		}
	}
	for _, key := range []string{"allOf", "anyOf", "oneOf", "prefixItems"} {
		if list, ok := schema[key].([]any); ok {
			for _, raw := range list {
				nested, ok := raw.(map[string]any)
				if !ok {
					return errors.New("schema composition entries must be objects")
				}
				if err := validateSchemaPolicy(nested, false); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func schemaMayDescribeObject(schema map[string]any) bool {
	if _, hasProperties := schema["properties"]; hasProperties {
		return true
	}
	if _, hasAdditionalProperties := schema["additionalProperties"]; hasAdditionalProperties {
		return true
	}
	switch typ := schema["type"].(type) {
	case string:
		return typ == "object"
	case []any:
		for _, value := range typ {
			if value == "object" {
				return true
			}
		}
	}
	return false
}

func isSensitiveFieldName(name string) bool {
	name = strings.ToLower(strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, name))
	for _, marker := range []string{"password", "passwd", "token", "secret", "credential", "privatekey", "apikey", "accesskey"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

func validateSecretReferenceValue(value any) error {
	text, ok := value.(string)
	if !ok || len(text) > 1024 || strings.ContainsAny(text, "\r\n\x00") {
		return errors.New("must be a Vaultix or SOPS secret reference")
	}
	parsed, err := url.Parse(text)
	if err != nil || (parsed.Scheme != "vaultix" && parsed.Scheme != "sops") || parsed.Opaque != "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Host == "" && parsed.Path == "") {
		return errors.New("must be a Vaultix or SOPS secret reference")
	}
	return nil
}
