import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import Ajv2020 from "ajv/dist/2020.js";
import addFormats from "ajv-formats";
import yaml from "js-yaml";

const schema = JSON.parse(fs.readFileSync(new URL("../../api/openapi/schemas/service-catalog.json", import.meta.url), "utf8"));
const ajv = new Ajv2020({ allErrors: true, strict: true });
addFormats(ajv);
const common = yaml.load(fs.readFileSync(new URL("../../api/openapi/schemas/common.yaml", import.meta.url), "utf8"));
ajv.addSchema(common.Uuid, "common.yaml#/Uuid");
ajv.addSchema(common.Timestamp, "common.yaml#/Timestamp");
ajv.addSchema(schema, "service-catalog.json");
const definition = ajv.getSchema("service-catalog.json#/$defs/ServiceDefinition");
const attachment = ajv.getSchema("service-catalog.json#/$defs/ServiceAttachment");
const tenant = "11111111-1111-4111-8111-111111111111";
const project = "22222222-2222-4222-8222-222222222222";
const instance = "33333333-3333-4333-8333-333333333333";
const base = { id: instance, definition_key: "example.postgres", desired_state: "enabled", observed_state: "unknown" };

test("service definitions separate capability, catalog state and allowed scopes", () => {
  const item = { key: "example.postgres", capability: "database.postgresql", display_name: "Example Database", supported_scopes: ["tenant", "project"], availability: "planned" };
  assert.equal(definition(item), true, JSON.stringify(definition.errors));
  for (const bad of [
    { ...item, key: "Example/Postgres" },
    { ...item, supported_scopes: ["project", "project"] },
    { ...item, supported_scopes: ["global"] },
    { ...item, availability: "healthy" },
    { ...item, api_key: "must-not-appear" }
  ]) assert.equal(definition(bad), false, JSON.stringify(bad));
});

test("attachments require explicit scope and reject undeclared credential fields", () => {
  const scopes = [
    { kind: "platform" },
    { kind: "tenant", tenant_id: tenant },
    { kind: "project", tenant_id: tenant, project_id: project },
    { kind: "workspace", tenant_id: tenant, workspace_id: project }
  ];
  for (const scope of scopes) assert.equal(attachment({ ...base, scope }), true, JSON.stringify(attachment.errors));
  for (const scope of [
    { kind: "tenant" },
    { kind: "project", project_id: project },
    { kind: "workspace", workspace_id: project },
    { kind: "platform", tenant_id: tenant },
    { kind: "tenant", tenant_id: "not-a-uuid" },
    { kind: "project", tenant_id: tenant, project_id: project, workspace_id: project }
  ]) assert.equal(attachment({ ...base, scope }), false, JSON.stringify(scope));
  assert.equal(attachment({ ...base, scope: scopes[1], password: "not-allowed" }), false);
  assert.equal(attachment({ ...base, scope: scopes[1], endpoint: "https://example.invalid" }), false);
});

test("observed state needs a timestamp without confusing desired and actual state", () => {
  const item = { ...base, scope: { kind: "tenant", tenant_id: tenant }, observed_state: "ready" };
  assert.equal(attachment(item), false);
  assert.equal(attachment({ ...item, observed_at: "2026-10-30T12:00:00Z" }), true, JSON.stringify(attachment.errors));
  assert.equal(attachment({ ...item, observed_at: "not-a-time" }), false);
  assert.equal(attachment({ ...item, observed_at: "2026-02-31T12:00:00Z" }), false);
  assert.equal(attachment({ ...item, observed_at: "2026-10-30T12:00:00Z", desired_state: "ready" }), false);
  assert.equal(attachment({ ...item, observed_at: "2026-10-30T12:00:00Z", observed_state: "healthy" }), false);
});


test("OpenAPI references the canonical service schema", () => {
  const openApiUrl = new URL("../../api/openapi/openapi.yaml", import.meta.url);
  const spec = yaml.load(fs.readFileSync(openApiUrl, "utf8"));
  for (const name of ["ServiceDefinition", "ServiceAttachment"]) {
    const ref = spec.components.schemas[name].$ref;
    const [file, pointer] = ref.split("#");
    const resolved = JSON.parse(fs.readFileSync(new URL(file, openApiUrl), "utf8"));
    let target = resolved;
    for (const token of pointer.slice(1).split("/")) target = target[token.replace(/~1/g, "/").replace(/~0/g, "~")];
    assert.deepEqual(target, schema.$defs[name]);
  }
});
