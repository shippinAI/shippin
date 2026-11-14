// Isolated real-platform acceptance. No route interception, canned API or session forgery.
// Phases: bootstrap -> restart panel with SHIPPIN_DEV_ACCOUNTS -> prepare -> activate bridge -> verify.
// Required: RUNNER_TEST_ISOLATED=1, RUNNER_TEST_ORIGIN (loopback HTTPS), RUNNER_TEST_PASSWORD,
// RUNNER_TEST_IDS_FILE, RUNNER_TEST_PLAYWRIGHT_ROOT (directory containing installed package.json).
// Optional fixture TLS: RUNNER_TEST_SELF_SIGNED=1. Never point this at an operator installation.
import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { readFile, writeFile } from "node:fs/promises";
import { resolve } from "node:path";
import { randomUUID } from "node:crypto";

function required(name) {
  const value = process.env[name];
  if (!value) throw new Error("Missing " + name);
  return value;
}
assert.equal(required("RUNNER_TEST_ISOLATED"), "1", "Explicit isolated test opt-in required");
const origin = new URL(required("RUNNER_TEST_ORIGIN"));
assert.equal(origin.protocol, "https:");
assert.ok(["127.0.0.1", "localhost", "[::1]"].includes(origin.hostname), "Only loopback fixture origins are allowed");
assert.equal(origin.pathname, "/");
assert.ok(!origin.username && !origin.password && !origin.search && !origin.hash);
const baseURL = origin.origin;
const password = required("RUNNER_TEST_PASSWORD");
const idsFile = required("RUNNER_TEST_IDS_FILE");
const phase = process.argv[2];
assert.ok(["bootstrap", "prepare", "reader-prepare", "configuration", "verify"].includes(phase));
const load = createRequire(resolve(required("RUNNER_TEST_PLAYWRIGHT_ROOT"), "package.json"));
const { chromium, request } = load("playwright");
const tls = process.env.RUNNER_TEST_SELF_SIGNED === "1";
const headers = { Origin: baseURL, "Sec-Fetch-Site": "same-origin" };
const identities = {
  admin: process.env.RUNNER_TEST_ADMIN ?? "admin@example.test",
  user: process.env.RUNNER_TEST_USER ?? "user@example.test",
  denied: process.env.RUNNER_TEST_DENIED ?? "denied@example.test",
  reader: process.env.RUNNER_TEST_READER ?? "reader@example.test",
};
for (const name of Object.values(identities)) assert.ok(name.endsWith("@example.test"), "Only test identities allowed");
function status(response, expected, action) {
  assert.equal(response.status(), expected, action + " returned unexpected HTTP status");
}
async function apiSession(identifier) {
  const api = await request.newContext({ baseURL, ignoreHTTPSErrors: tls, extraHTTPHeaders: headers });
  const login = await api.post("/v1/session/password", { data: { identifier, password } });
  status(login, 200, "Fixture password login");
  return api;
}
if (phase === "bootstrap") {
  const api = await request.newContext({ baseURL, ignoreHTTPSErrors: tls, extraHTTPHeaders: headers });
  try {
    const result = await api.post("/v1/setup", { data: {
      Code: required("RUNNER_TEST_SETUP_CODE"), Password: password,
      Organization: "Runner qualification operations",
      Brand: { name: "Runner qualification", short: "Qualification", tagline: "Disposable integration test" },
    } });
    status(result, 200, "Synthetic installation setup");
    console.log("PASS bootstrap: real setup API completed on disposable database; restart panel to seed development accounts.");
  } finally { await api.dispose(); }
} else if (phase === "prepare" || phase === "reader-prepare") {
  let fixtures = {};
  try { fixtures = JSON.parse(await readFile(idsFile, "utf8")); } catch (error) { if (error.code !== "ENOENT") throw error; }
  for (const [kind, identifier] of Object.entries(identities)) {
    if ((phase === "reader-prepare" && kind !== "reader") || fixtures[kind]) continue;
    const api = await apiSession(identifier);
    try {
      const meResponse = await api.get("/v1/me");
      status(meResponse, 200, "Fixture identity");
      const me = await meResponse.json();
      const fixture = { subject: me.user_id, tenant: me.tenants?.[0]?.tenant_id ?? "" };
      assert.equal(typeof fixture.subject, "string");
      if (kind !== "admin") {
        assert.ok(fixture.tenant, "Customer fixture must have an organization");
        const created = await api.post("/v1/tenants/" + fixture.tenant + "/projects", {
          headers: { "Idempotency-Key": randomUUID() },
          data: { name: "runner-qualification-" + randomUUID().slice(0, 8), environment: "dev" },
        });
        status(created, 201, "Create fixture project");
        const project = await created.json();
        fixture.project = project.id;
        assert.equal(typeof fixture.project, "string");
      }
      fixtures[kind] = fixture;
    } finally { await api.dispose(); }
  }
  await writeFile(idsFile, JSON.stringify(fixtures, null, 2), { mode: 0o600 });
  console.log("PASS prepare: real admin/customer logins and customer project creation; only subject/tenant/project IDs saved.");
} else if (phase === "configuration") {
  const { qualifyProfiles } = await import("./test-runner-profile.mjs");
  await qualifyProfiles({ chromium, baseURL, tls, identities, password, idsFile });
} else {
  const ids = JSON.parse(await readFile(idsFile, "utf8"));
  const browser = await chromium.launch({ headless: true });
  const checks = [];
  async function loggedIn(kind) {
    const context = await browser.newContext({ baseURL, ignoreHTTPSErrors: tls });
    const page = await context.newPage();
    await page.goto("/login");
    await page.getByRole("textbox", { name: "Username or email", exact: true }).fill(identities[kind]);
    await page.getByLabel("Password", { exact: true }).fill(password);
    const responsePromise = page.waitForResponse((r) => new URL(r.url()).pathname === "/v1/session/password" && r.request().method() === "POST");
    await page.getByRole("button", { name: "Sign in with password", exact: true }).click();
    status(await responsePromise, 200, "Browser fixture sign-in");
    await page.waitForURL((url) => url.pathname !== "/login");
    return { context, page };
  }
  try {
    const anonymous = await browser.newContext({ baseURL, ignoreHTTPSErrors: tls });
    const anonymousResponse = await anonymous.request.get("/v1/runner/projects");
    assert.ok([401, 403].includes(anonymousResponse.status()), "Anonymous discovery must be denied");
    await anonymous.close();
    checks.push("anonymous discovery denied");
    for (const kind of ["admin", "denied"]) {
      const { context, page } = await loggedIn(kind);
      try {
        const discovery = await context.request.get("/v1/runner/projects");
        status(discovery, 200, "Unassigned fixture discovery");
        assert.deepEqual((await discovery.json()).projects ?? [], [], "Staff status is not a runner grant");
        const forbidden = await context.request.get("/v1/projects/" + ids.user.project + "/runs");
        assert.ok([401, 403, 404].includes(forbidden.status()), "Cross-project runner access must be denied");
        if (kind === "denied") {
          await page.goto("/t/" + ids.denied.tenant + "/runners/" + ids.user.project + "/runs");
          await page.getByText("Your current session does not have access to this project or section.", { exact: true }).waitFor();
        }
        checks.push(kind + " has no runner grant and cannot read another project");
      } finally { await context.close(); }
    }
    const { context, page } = await loggedIn("user");
    try {
      const discovered = await context.request.get("/v1/runner/projects");
      status(discovered, 200, "Authorized discovery");
      const projects = (await discovered.json()).projects;
      assert.ok(projects.some((p) => p.id === ids.user.project && p.tenant_id === ids.user.tenant));
      assert.ok(!projects.some((p) => p.id === ids.denied.project));
      const prefix = "/t/" + ids.user.tenant + "/runners";
      await page.goto(prefix + "/" + ids.user.project + "/overview");
      await page.getByRole("heading", { name: "Runners", exact: true }).waitFor();
      const runResponse = page.waitForResponse((r) => new URL(r.url()).pathname === "/v1/projects/" + ids.user.project + "/runs");
      await page.getByRole("tab", { name: "Runs", exact: true }).click();
      const runsHTTP = await runResponse;
      status(runsHTTP, 200, "Real runner runs through platform gateway");
      const runs = await runsHTTP.json();
      const forbiddenKeys = /^(?:token|access_token|refresh_token|authorization|cookie|set-cookie|signing_key|private_key|password|secret|credential)$/i;
      function assertSafeKeys(value) {
        if (value === null || typeof value !== "object") return;
        for (const [key, item] of Object.entries(value)) {
          assert.ok(!forbiddenKeys.test(key), "Runner response contains a prohibited credential field");
          assertSafeKeys(item);
        }
      }
      assertSafeKeys(runs);
      if (process.env.RUNNER_TEST_EXPECT_RUN_ID) {
        const expected = process.env.RUNNER_TEST_EXPECT_RUN_ID;
        assert.ok(runs.items?.some((r) => r.id === expected && r.project === ids.user.project), "Persisted fixture run must cross real gateway");
        await page.getByRole("cell", { name: expected, exact: true }).waitFor();
        const cancelPath = "/v1/projects/" + ids.user.project + "/runs/" + expected + "/cancel";
        const foreignCancel = await context.request.post(cancelPath, {
          headers: { Origin: "https://cross-origin.example.invalid", "Sec-Fetch-Site": "cross-site" },
          data: {},
        });
        status(foreignCancel, 403, "Cross-origin runner cancellation");
        const unauthorizedCancel = await context.request.post(cancelPath, {
          headers, data: {},
        });
        assert.ok([401, 403].includes(unauthorizedCancel.status()), "Read-only grant must not cancel a run");
        const afterCancel = await context.request.get("/v1/projects/" + ids.user.project + "/runs");
        status(afterCancel, 200, "Read after denied cancellations");
        const afterRuns = await afterCancel.json();
        const before = runs.items.find((r) => r.id === expected);
        const after = afterRuns.items.find((r) => r.id === expected);
        assert.ok(after, "Persisted run still exists");
        assert.equal(after.cancel_requested, before.cancel_requested, "Denied cancellation must not mutate the run");
        assert.equal(after.state, before.state, "Denied cancellation must not change state");
        assertSafeKeys(afterRuns);
        checks.push("runner cancellation denied cross-origin and without write grant, with unchanged persisted state");
        checks.push("persisted fixture run rendered through real control store and gateway; response contains no credential fields");
      } else {
        assert.equal((runs.items ?? []).length, 0, "Initial qualification expects no seeded runs");
        await page.getByText("No recorded runs for this project. This does not confirm that a runner pool is ready.", { exact: true }).waitFor();
      }
      await page.getByText("Run records are eventually consistent and may lag live execution.", { exact: true }).waitFor();
      assert.equal(new URL(page.url()).pathname, prefix + "/" + ids.user.project + "/runs");
      const connectionResponse = page.waitForResponse((r) => new URL(r.url()).pathname === "/v1/projects/" + ids.user.project + "/connectors");
      await page.getByRole("tab", { name: "Connections", exact: true }).click();
      status(await connectionResponse, 200, "Real runner connections through platform gateway");
      await page.getByText("Enrollment and connection state do not prove available capacity or a successful job.", { exact: true }).waitFor();
      await page.getByRole("tab", { name: "Configuration", exact: true }).click();
      await page.getByText("Credential rotation is not available in this panel. No secrets are displayed or accepted here.", { exact: true }).waitFor();
      assert.equal(await page.locator('input[type="password"]').count(), 0);
      await page.reload();
      await page.getByRole("tab", { name: "Configuration", exact: true, selected: true }).waitFor();
      checks.push("authorized user traverses real discovery, runs, connections and URL-persistent configuration tabs");
      const crossOrigin = await context.request.post("/v1/session/password", {
        headers: { Origin: "https://cross-origin.example.invalid", "Sec-Fetch-Site": "cross-site" },
        data: { identifier: identities.user, password },
      });
      status(crossOrigin, 403, "Cross-origin write");
      checks.push("cross-origin write refused");
    } finally { await context.close(); }
    for (const check of checks) console.log("PASS " + check);
    console.log("PASS isolated browser qualification. This does not prove a live owner grant, deployment or running worker pool.");
  } finally { await browser.close(); }
}
