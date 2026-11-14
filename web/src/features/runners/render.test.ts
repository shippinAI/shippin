import assert from "node:assert/strict";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RunnerResource } from "./ui/runner-resource";
import type { RunnerProject } from "./model";

const project: RunnerProject = { id: "project-1", tenant_id: "org-a", name: "One", scopes: ["runs:read"] };
function render(resource: "runs" | "connectors", data: unknown, target = project) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(["runner", "org-a", target.id, resource, target.scopes], data);
  const markup = renderToStaticMarkup(createElement(QueryClientProvider, { client }, createElement(RunnerResource, { tenant: "org-a", project: target, resource })));
  client.clear();
  return markup;
}
const runs = render("runs", { items: [{ id: "run-123", project: "project-1", repository: "org/repo", profile: "small", state: "queued", projected_at: "2026-11-14T00:00:00Z", cancel_requested: true }], consistency: "eventual", next_cursor: "opaque-value" });
assert.match(runs, /run-123/);
assert.match(runs, /queued/);
assert.match(runs, /Cancellation requested/);
assert.match(runs, /limited snapshot/);
assert.doesNotMatch(runs, /opaque-value/);
const denied = render("connectors", { github: { state: "should-not-display" } });
assert.match(denied, /does not have access/);
assert.doesNotMatch(denied, /should-not-display/);
const empty = render("runs", { items: null, consistency: "eventual" });
assert.match(empty, /does not confirm that a runner pool is ready/);
const connections = render("connectors", { github: { state: "unconfigured" }, execution_hosts: { state: "empty", workers: [], truncated: false }, secret: "must-not-render" }, { ...project, scopes: ["workers:manage"] });
assert.match(connections, /unconfigured/);
assert.match(connections, /No execution hosts/);
assert.doesNotMatch(connections, /must-not-render/);
console.log("Runner component rendering passed: recorded states, bounded snapshots, scope denial, empty capacity and no arbitrary secret rendering.");

const { ProfileForm } = await import("./ui/runner-configuration");
const { apiFixtureSchema } = await import("./profiles.test-fixture");
const readProject = { ...project, scopes: ["profiles:read"], profiles: ["small"] };
const client = new QueryClient();
const profileProps = { tenant: "org-a", project: readProject, profile: "small", schema: apiFixtureSchema, template: null, reload: () => {} };
const readProfile = renderToStaticMarkup(createElement(QueryClientProvider, { client }, createElement(ProfileForm, profileProps)));
assert.match(readProfile, /Your profile access is read-only/);
assert.doesNotMatch(readProfile, />Save profile revision</);
assert.match(readProfile, /Worker application is unreported/);
assert.match(readProfile, /schema default: 4096/);
const writableProfile = renderToStaticMarkup(createElement(QueryClientProvider, { client }, createElement(ProfileForm, { ...profileProps, project: { ...readProject, scopes: ["profiles:read", "profiles:write"] } })));
assert.match(writableProfile, />Save profile revision</);
assert.match(writableProfile, /Complete required fields/);
assert.doesNotMatch(writableProfile, /type="password"/);
client.clear();
console.log("Profile component render passed: readonly controls, unverified/applied distinction, defaults, typed required fields and no secret inputs.");
