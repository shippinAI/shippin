import assert from "node:assert/strict";
import { apiFixtureSchema } from "./profiles.test-fixture";
import { draftOf, grantedProfiles, loadProfile, profilePayload, saveProfile } from "./profiles";
import type { RunnerProject } from "./model";
const project: RunnerProject = { id: "p1", tenant_id: "t1", name: "Fixture", scopes: ["profiles:read", "profiles:write"], profiles: ["small"] };
const schema = apiFixtureSchema;
const draft = { image_sha256: "a".repeat(64), kernel_sha256: "b".repeat(64), runner_version: "2.330.0" };
assert.deepEqual(grantedProfiles({ ...project, scopes: [] }), []);
assert.deepEqual(profilePayload(schema, draft), { body: draft, errors: {} }, "Blank optional values stay omitted, not flattened defaults");
assert.equal(profilePayload(schema, { ...draft, vcpu_count: "3" }).errors.vcpu_count, "bounds");
assert.equal(profilePayload(schema, { ...draft, memory_mib: "256.5" }).errors.memory_mib, "bounds");
assert.equal(profilePayload(schema, { ...draft, image_sha256: "secret" }).errors.image_sha256, "format");
assert.ok(!("password" in profilePayload(schema, { ...draft, password: "never send" }).body));
assert.deepEqual(draftOf(null), {});
let calls: { path: string; body?: string }[] = [];
let status = 200;
let reply: unknown;
const original = globalThis.fetch;
globalThis.fetch = (async (path, init) => {
  calls.push({ path: String(path), body: init?.body as string });
  if (String(path).endsWith("/profiles/schema")) return new Response(JSON.stringify(schema), { status: 200 });
  return new Response(JSON.stringify(reply), { status });
}) as typeof fetch;
try {
  status = 404; reply = {};
  assert.equal((await loadProfile("t1", project, "small")).template, null);
  assert.equal(calls.length, 2);
  calls = [];
  await assert.rejects(saveProfile("t1", { ...project, scopes: ["profiles:read"] }, "small", 0, schema, draft));
  await assert.rejects(loadProfile("t2", project, "small"));
  await assert.rejects(loadProfile("t1", project, "not-granted"));
  assert.equal(calls.length, 0, "Denied inputs must not reach network");
  status = 200; reply = { id: "small", project: "p1", revision: 1, schema_version: 1, template: draft, worker_applied: "unreported" };
  assert.equal((await saveProfile("t1", project, "small", 0, schema, { ...draft, password: "never send" })).revision, 1);
  assert.equal(calls[0].path, "/v1/projects/p1/profile-templates/small/revisions/0");
  assert.deepEqual(JSON.parse(calls[0].body!), draft);
  const validReply = reply;
  for (const invalid of [null, [], { ...draft, secret: "not allowed" }, { ...draft, vcpu_count: "2" }, { ...draft, image_sha256: "invalid" }]) {
    reply = { ...(validReply as object), template: invalid };
    await assert.rejects(loadProfile("t1", project, "small"), /Invalid template/);
  }
  reply = validReply;
  const beforeConflict = calls.length;
  assert.deepEqual(grantedProfiles({ ...project, profiles: ["small", "small", "../bad"] }), ["small"]);
  status = 409;
  await assert.rejects(saveProfile("t1", project, "small", 1, schema, draft), (e: any) => e.status === 409);
  assert.equal(calls.length, beforeConflict + 1, "No automatic retry on stale revision");
  status = 503;
  await assert.rejects(saveProfile("t1", project, "small", 1, schema, draft));
  assert.equal(calls.length, beforeConflict + 2, "No retry on uncertain outcome");
} finally { globalThis.fetch = original; }
console.log("Profile client tests passed: schema bounds/defaults, exact scoped requests, no credentials, creation and no-retry conflict semantics.");
