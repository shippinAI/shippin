import test from "node:test";
import assert from "node:assert/strict";
import { getCapabilityFromOperation, getPathParameters, hasOperatorRole, isCapabilityAllowed, isOperationAllowedForMode } from "../src/features/workbench/operations.ts";

const operation = { method: "POST", path: "/tenants/{tenantId}/workspaces/{workspaceId}", operationId: "createWorkspace", tags: ["workspaces"] };
const operator = { platform_roles: ["op_admin"], tenants: [], capabilities: { "workspace:create": false } };

test("route visibility distinguishes customer operations and operator roles", () => {
  assert.equal(isOperationAllowedForMode(operation, "customer", null), true);
  assert.equal(isOperationAllowedForMode(operation, "operator", operator), false);
  const adminOperation = { ...operation, tags: ["tenants"] };
  assert.equal(isOperationAllowedForMode(adminOperation, "operator", null), false);
  assert.equal(isOperationAllowedForMode(adminOperation, "operator", operator), true);
  assert.equal(hasOperatorRole({ platform_roles: ["member"], tenants: [] }), false);
});

test("parameters and capability hints follow the workbench contract", () => {
  assert.deepEqual(getPathParameters(operation.path), ["tenantId", "workspaceId"]);
  assert.deepEqual(getPathParameters("/me"), []);
  const config = { capabilityHints: { createWorkspace: "workspace:create" } };
  assert.equal(getCapabilityFromOperation(operation, config), "workspace:create");
  assert.equal(isCapabilityAllowed(operation, config, operator), false);
  assert.equal(isCapabilityAllowed(operation, config, null), true);
  assert.equal(getCapabilityFromOperation(operation, null), "workspaces:create");
});
