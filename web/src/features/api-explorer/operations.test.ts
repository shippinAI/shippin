import assert from "node:assert/strict";
import { availableOperations, bodyError, hasBody, matchOperations, pathParameters, resolvePath, type ApiOperation } from "./operations";

const op = (path: string, method = "GET", summary = "", tags = ["x"]): ApiOperation => ({ method, path, operationId: `${method}:${path}`, summary, tags, implementationStatus: "registered" });

assert.deepEqual(pathParameters("/tenants/{tenantId}/workspaces/{workspaceId}"), ["tenantId", "workspaceId"]);
assert.deepEqual(pathParameters("/me"), []);

const path = "/tenants/{tenantId}/workspaces/{workspaceId}";
assert.equal(resolvePath(path, { tenantId: "t-1", workspaceId: "my workspace" }), "/tenants/t-1/workspaces/my%20workspace");
for (const unsafe of [".", "..", "x/y", "x\\y", "%2e%2e", "foo?admin=true", "x#frag", "\u0000"]) {
  assert.throws(() => resolvePath(path, { tenantId: unsafe, workspaceId: "ok" }), /Invalid path parameter/, `accepted ${JSON.stringify(unsafe)}`);
}
assert.throws(() => resolvePath(path, { tenantId: "t-1" }), /Missing required path parameter: workspaceId/);
assert.throws(() => resolvePath(path, { tenantId: "   ", workspaceId: "ok" }), /Missing required path parameter: tenantId/);

const ops = [op("/me", "GET", "Who am I", ["me"]), op("/tenants", "POST", "Create a tenant", ["tenants"]), op("/tenants", "GET", "List tenants", ["tenants"])];
assert.equal(matchOperations(ops, "").length, 3);
assert.deepEqual(matchOperations(ops, "post tenant").map((o) => o.operationId), ["POST:/tenants"], "every word must match");
assert.deepEqual(matchOperations(ops, "WHO").map((o) => o.path), ["/me"]);
assert.equal(matchOperations(ops, "nothing here").length, 0);

assert.equal(hasBody("get"), false);
assert.equal(hasBody("POST"), true);
assert.equal(bodyError(""), null);
assert.equal(bodyError('{"a":1}'), null);
assert.match(bodyError("{a:1}") ?? "", /not valid JSON/);

console.log("api-explorer operations ok");

// A route the contract ties to a feature is offered only with it; an unknown capability counts as off.
const featured: ApiOperation[] = [{ ...op("/me") }, { ...op("/chat/sessions"), feature: "feature.chat" }, { ...op("/x/secrets"), feature: "feature.secrets" }];
const offered = (caps: Record<string, boolean>) => availableOperations(featured, caps).map((o) => o.path);
assert.deepEqual(offered({}), ["/me"]);
assert.deepEqual(offered({ "feature.chat": true }), ["/me", "/chat/sessions"]);
assert.deepEqual(offered({ "feature.chat": false, "feature.secrets": true }), ["/me", "/x/secrets"]);
