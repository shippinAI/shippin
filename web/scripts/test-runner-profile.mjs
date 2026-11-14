// Real browser/API profile acceptance. Only invoked by loopback-only isolated harness.
import assert from "node:assert/strict";
import { readFile, writeFile } from "node:fs/promises";
export async function qualifyProfiles({ chromium, baseURL, tls, identities, password, idsFile }) {
  const ids = JSON.parse(await readFile(idsFile, "utf8"));
  const profile = process.env.RUNNER_TEST_PROFILE ?? "fixture-profile";
  const readerProfile = process.env.RUNNER_TEST_READER_PROFILE ?? "read-only-profile";
  const browser = await chromium.launch({ headless: true });
  const headers = { Origin: baseURL, "Sec-Fetch-Site": "same-origin" };
  async function login(kind) {
    const context = await browser.newContext({ baseURL, ignoreHTTPSErrors: tls });
    const page = await context.newPage();
    await page.goto("/login");
    await page.getByRole("textbox", { name: "Username or email", exact: true }).fill(identities[kind]);
    await page.getByLabel("Password", { exact: true }).fill(password);
    const response = page.waitForResponse((r) => new URL(r.url()).pathname === "/v1/session/password");
    await page.getByRole("button", { name: "Sign in with password", exact: true }).click();
    assert.equal((await response).status(), 200, "Actual browser profile fixture login");
    await page.waitForURL((url) => url.pathname !== "/login");
    return { context, page };
  }
  const resultFile = process.env.RUNNER_TEST_PROFILE_RESULT_FILE ?? idsFile + ".profile.json";
  try {
    const { context, page } = await login("user");
    try {
      const fixture = ids.user;
      const path = "/v1/projects/" + fixture.project + "/profile-templates/" + profile;
      const ui = "/t/" + fixture.tenant + "/runners/" + fixture.project + "/configuration";
      await page.goto(ui);
      await page.getByLabel("Pinned root filesystem SHA-256", { exact: true }).waitFor();
      await page.getByText("Credential rotation is not available in this panel. No secrets are displayed or accepted here.", { exact: true }).waitFor();
      const snapshot = await context.request.get(path);
      assert.ok([200, 404].includes(snapshot.status()), "Readable existing or new profile");
      let template = snapshot.status() === 404 ? null : await snapshot.json();
      let revision = template?.revision ?? 0;
      if (process.env.RUNNER_TEST_PROFILE_PERSISTENCE === "1") {
        const expected = JSON.parse(await readFile(resultFile, "utf8"));
        assert.equal(revision, expected.revision);
        assert.deepEqual(template.template, expected.template);
        await page.getByText("Stored revision: " + revision, { exact: true }).waitFor();
        assert.equal(await page.getByLabel("Maximum attempt duration in seconds", { exact: true }).inputValue(), String(expected.template.timeout_seconds));
        console.log("PASS exact profile revision and values survived control recreation and render in real browser.");
      } else {
        await page.getByLabel("Pinned root filesystem SHA-256", { exact: true }).fill("a".repeat(64));
        await page.getByLabel("Pinned guest kernel SHA-256", { exact: true }).fill("b".repeat(64));
        await page.getByLabel("Pinned GitHub runner version", { exact: true }).fill("2.330.0");
        await page.getByLabel("Maximum attempt duration in seconds", { exact: true }).fill("100");
        async function save(expectedStatus, expectedRevision) {
          const pending = page.waitForResponse((r) => new URL(r.url()).pathname === path + "/revisions/" + expectedRevision && r.request().method() === "POST");
          await page.getByRole("button", { name: "Save profile revision", exact: true }).click();
          const response = await pending;
          assert.equal(response.status(), expectedStatus, "Actual profile UI save");
          return response;
        }
        template = await (await save(200, revision)).json();
        assert.equal(template.revision, revision + 1);
        revision = template.revision;
        await page.getByText("Revision saved. Worker application remains unreported.", { exact: true }).waitFor();
        await page.reload();
        await page.getByText("Stored revision: " + revision, { exact: true }).waitFor();
        assert.equal(await page.getByLabel("Maximum attempt duration in seconds", { exact: true }).inputValue(), "100");
        await page.getByLabel("Maximum attempt duration in seconds", { exact: true }).fill("120");
        const concurrent = await context.request.post(path + "/revisions/" + revision, { headers, data: { ...template.template, timeout_seconds: 150 } });
        assert.equal(concurrent.status(), 200, "Concurrent real versioned writer");
        const concurrentTemplate = await concurrent.json();
        await save(409, revision);
        await page.getByText(/The stored revision changed. Your edits are preserved/).waitFor();
        assert.equal(await page.getByLabel("Maximum attempt duration in seconds", { exact: true }).inputValue(), "120");
        assert.equal(await page.getByRole("button", { name: "Save profile revision", exact: true }).isDisabled(), true);
        const reload = page.waitForResponse((r) => new URL(r.url()).pathname === path && r.request().method() === "GET");
        await page.getByRole("button", { name: "Reload and discard edits", exact: true }).click();
        assert.equal((await reload).status(), 200);
        revision = concurrentTemplate.revision;
        await page.getByText("Stored revision: " + revision, { exact: true }).waitFor();
        assert.equal(await page.getByLabel("Maximum attempt duration in seconds", { exact: true }).inputValue(), "150");
        await page.getByLabel("Maximum attempt duration in seconds", { exact: true }).fill("140");
        template = await (await save(200, revision)).json();
        revision = template.revision;
        const badField = await context.request.post(path + "/revisions/" + revision, { headers, data: { ...template.template, secret: "rejected-fixture-only" } });
        assert.equal(badField.status(), 400, "Unknown secret field must be rejected");
        const ungranted = await context.request.post("/v1/projects/" + fixture.project + "/profile-templates/not-granted/revisions/0", { headers, data: template.template });
        assert.ok([401, 403, 404].includes(ungranted.status()), "Out-of-grant profile rejected");
        const final = await context.request.get(path);
        assert.equal(final.status(), 200);
        const actual = await final.json();
        assert.equal(actual.revision, revision, "Rejected writes must not increment revision");
        assert.deepEqual(actual.template, template.template);
        assert.equal(actual.worker_applied, "unreported");
        assert.equal(await page.locator('input[type="password"]').count(), 0);
        await writeFile(resultFile, JSON.stringify({ revision, template: actual.template }, null, 2), { mode: 0o600 });
        console.log("PASS real profile save/reload, concurrent CAS409 preserves edits, explicit reload, second save and rejected secret/out-of-grant writes.");
      }
    } finally { await context.close(); }
    const { context: readerContext, page: readerPage } = await login("reader");
    try {
      const fixture = ids.reader;
      const discovery = await readerContext.request.get("/v1/runner/projects");
      assert.equal(discovery.status(), 200);
      const projects = (await discovery.json()).projects;
      const grant = projects.find((p) => p.id === fixture.project);
      assert.ok(grant?.profiles.includes(readerProfile) && grant.scopes.includes("profiles:read") && !grant.scopes.includes("profiles:write"));
      await readerPage.goto("/t/" + fixture.tenant + "/runners/" + fixture.project + "/configuration");
      await readerPage.getByText("Your profile access is read-only.", { exact: true }).waitFor();
      const readerRead = await readerContext.request.get("/v1/projects/" + fixture.project + "/profile-templates/" + readerProfile);
      assert.equal(readerRead.status(), 200, "Reader must read an actual persisted fixture profile");
      const readerTemplate = await readerRead.json();
      assert.equal(await readerPage.getByLabel("Pinned root filesystem SHA-256", { exact: true }).inputValue(), readerTemplate.template.image_sha256);
      await readerPage.getByText("Stored revision: " + readerTemplate.revision, { exact: true }).waitFor();
      assert.equal(await readerPage.getByRole("button", { name: "Save profile revision", exact: true }).count(), 0);
      assert.equal(await readerPage.getByLabel("Pinned root filesystem SHA-256", { exact: true }).getAttribute("readonly"), "");
      const denied = await readerContext.request.post("/v1/projects/" + fixture.project + "/profile-templates/" + readerProfile + "/revisions/0", {
        headers, data: { image_sha256: "a".repeat(64), kernel_sha256: "b".repeat(64), runner_version: "2.330.0" },
      });
      assert.ok([401, 403].includes(denied.status()), "Read-only profile grant cannot write");
      console.log("PASS separate reader fixture has readonly UI and server-enforced write denial.");
    } finally { await readerContext.close(); }
    console.log("PASS profile configuration qualification; saved is not applied, no credential lifecycle claim.");
  } finally { await browser.close(); }
}
