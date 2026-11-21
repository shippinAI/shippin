// Disposable Vaultix for proving the panel's secret store (internal/platform/secret_store) against the
// real product rather than a fake. Local Docker only, images already present (--pull=never).
//
//   node deploy/vaultix/proof-secret-store.mjs up <vaultix-image> <env-file> [host]
//   node deploy/vaultix/proof-secret-store.mjs down
//
// up creates a private network, PostgreSQL, Dragonfly and the Vaultix app (published on a random
// 127.0.0.1 port), then through the API: the first admin and organization, a project (with its default
// environments, prod among them), a machine identity with Universal Auth and the project's member role,
// and a second identity with Universal Auth and no project access. Every credential is generated here
// for this run and written only to <env-file> (mode 0600) as SHIPPIN_VAULTIX_PROOF_* lines for
// internal/platform/secret_store/rig_test.go. [host] is the name the test reaches the app by
// (default host.docker.internal, for a test run in a container). Nothing is printed but ids and the port.
//
// down removes every container, volume and network labelled by this script (all runs).
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { randomBytes } from "node:crypto";
import { writeFileSync } from "node:fs";

const LABEL = "shippin.vxproof";
const PG = "postgres:16.6-alpine";
const CACHE = "docker.dragonflydb.io/dragonflydb/dragonfly:v1.34.2";

function docker(args, { allowFailure = false } = {}) {
  const r = spawnSync("docker", args, { timeout: 120_000, maxBuffer: 16 << 20, windowsHide: true });
  if (!allowFailure && (r.error || r.status !== 0)) throw new Error(`docker ${args[0]} ${args[1] ?? ""} failed: ${r.stderr?.toString().trim()}`);
  return r.stdout?.toString().trim() ?? "";
}
const pause = ms => new Promise(done => setTimeout(done, ms));
async function waitFor(test, what, timeout = 180_000) {
  for (const deadline = Date.now() + timeout; Date.now() < deadline; await pause(1000)) if (await test()) return;
  throw new Error(`timed out waiting for ${what}`);
}

function down() {
  const ids = docker(["ps", "-aq", "--filter", `label=${LABEL}`]).split(/\s+/).filter(Boolean);
  if (ids.length) docker(["rm", "-f", "-v", ...ids]);
  const nets = docker(["network", "ls", "-q", "--filter", `label=${LABEL}`]).split(/\s+/).filter(Boolean);
  if (nets.length) docker(["network", "rm", ...nets]);
  const vols = docker(["volume", "ls", "-q", "--filter", `label=${LABEL}`]).split(/\s+/).filter(Boolean);
  if (vols.length) docker(["volume", "rm", ...vols]);
  console.log(JSON.stringify({ removed: { containers: ids.length, networks: nets.length, volumes: vols.length } }));
}

async function up(image, envFile, host = "host.docker.internal") {
  assert.ok(image && envFile, "usage: up <vaultix-image> <env-file> [host]");
  const title = docker(["image", "inspect", "--format", '{{index .Config.Labels "org.opencontainers.image.title"}}', image]);
  assert.equal(title, "Vaultix", "not a Vaultix image");
  const run = `shippin-vxproof-${randomBytes(6).toString("hex")}`;
  const label = ["--label", `${LABEL}=${run}`];
  docker(["network", "create", ...label, run]);
  const dbPassword = randomBytes(24).toString("hex");
  const create = (name, img, env, args = [], extra = []) => {
    const id = docker(["create", "--pull=never", "--init", ...label, "--name", name, "--network", run,
      ...extra, ...Object.entries(env).flatMap(([k, v]) => ["-e", `${k}=${v}`]), img, ...args]);
    docker(["start", id]);
    return id;
  };
  const db = create(`${run}-db`, PG, { POSTGRES_USER: "vaultix", POSTGRES_DB: "vaultix", POSTGRES_PASSWORD: dbPassword });
  const cache = create(`${run}-cache`, CACHE, {},
    ["--default_lua_flags=allow-undeclared-keys", "--cache_mode=false", "--maxmemory=512mb", "--proactor_threads=2"]);
  await waitFor(() => spawnSync("docker", ["exec", db, "pg_isready", "-U", "vaultix", "-d", "vaultix"]).status === 0, "the database");
  await waitFor(() => spawnSync("docker", ["exec", cache, "redis-cli", "ping"]).status === 0, "the cache");
  const app = create(`${run}-app`, image, {
    DB_CONNECTION_URI: `postgres://vaultix:${dbPassword}@${run}-db:5432/vaultix`,
    VAULTIX_CACHE_PROVIDER: "dragonfly", VAULTIX_CACHE_URL: `redis://${run}-cache:6379`, REDIS_URL: `redis://${run}-cache:6379`,
    ENCRYPTION_KEY: randomBytes(16).toString("hex"), AUTH_SECRET: randomBytes(32).toString("hex"),
    SITE_URL: "http://127.0.0.1:8080", TELEMETRY_ENABLED: "false", DISABLE_UPDATE_CHECK: "true"
  }, [], ["-p", "127.0.0.1::8080"]);
  const port = docker(["port", app, "8080/tcp"]).split("\n")[0].split(":").pop();
  const base = `http://127.0.0.1:${port}`;
  console.log(JSON.stringify({ run, port }));

  async function api(path, { method = "GET", body, token, status = 200 } = {}) {
    const r = await fetch(base + path, { method, signal: AbortSignal.timeout(30_000),
      headers: { "Content-Type": "application/json", ...(token ? { Authorization: `Bearer ${token}` } : {}) },
      ...(body === undefined ? {} : { body: JSON.stringify(body) }) });
    const text = await r.text();
    // Error bodies name the problem; they never carry the values sent.
    if (r.status !== status) throw new Error(`${method} ${path.split("?")[0]}: ${r.status} ${text.slice(0, 300)}`);
    return text ? JSON.parse(text) : {};
  }
  await waitFor(async () => (await fetch(base + "/api/status").catch(() => null))?.status === 200, "the Vaultix app");

  const password = `Vx9!${randomBytes(24).toString("hex")}`;
  const owner = await api("/api/v1/admin/signup", { method: "POST",
    body: { email: `${run}@vaultix.invalid`, password, firstName: "Proof", lastName: "Admin", organizationName: "Shippin proof" } });
  const org = owner.organization.id;
  const { token: admin } = await api("/api/v3/auth/select-organization", { method: "POST", token: owner.token, body: { organizationId: org } });
  const { project } = await api("/api/v1/projects", { method: "POST", token: admin, body: { projectName: "Shippin panel" } });
  const envs = (project.environments ?? []).map(e => e.slug);
  assert.ok(envs.includes("prod"), `the project has no prod environment: ${envs}`);

  async function identity(name, member) {
    const { identity } = await api("/api/v1/identities", { method: "POST", token: admin, body: { name, organizationId: org, role: "no-access" } });
    if (member) await api(`/api/v1/projects/${project.id}/memberships/identities/${identity.id}`, { method: "POST", token: admin,
      body: { roles: [{ role: "member", isTemporary: false }] } });
    const ua = await api(`/api/v1/auth/universal-auth/identities/${identity.id}`, { method: "POST", token: admin,
      body: { accessTokenTTL: 3600, accessTokenMaxTTL: 3600, clientSecretTrustedIps: [{ ipAddress: "0.0.0.0/0" }, { ipAddress: "::/0" }],
        accessTokenTrustedIps: [{ ipAddress: "0.0.0.0/0" }, { ipAddress: "::/0" }] } });
    const cs = await api(`/api/v1/auth/universal-auth/identities/${identity.id}/client-secrets`, { method: "POST", token: admin,
      body: { description: "Shippin secret store proof", ttl: 0, numUsesLimit: 0 } });
    return { id: identity.id, clientId: ua.identityUniversalAuth.clientId, clientSecret: cs.clientSecret };
  }
  const writer = await identity("Shippin panel", true);
  const outsider = await identity("No project access", false);

  const lines = {
    SHIPPIN_VAULTIX_PROOF_URL: `http://${host}:${port}`,
    SHIPPIN_VAULTIX_PROOF_PROJECT: project.id,
    SHIPPIN_VAULTIX_PROOF_ENV: "prod",
    SHIPPIN_VAULTIX_PROOF_CLIENT_ID: writer.clientId,
    SHIPPIN_VAULTIX_PROOF_CLIENT_SECRET: writer.clientSecret,
    SHIPPIN_VAULTIX_PROOF_OUTSIDER_CLIENT_ID: outsider.clientId,
    SHIPPIN_VAULTIX_PROOF_OUTSIDER_CLIENT_SECRET: outsider.clientSecret
  };
  writeFileSync(envFile, Object.entries(lines).map(([k, v]) => `${k}=${v}`).join("\n") + "\n", { mode: 0o600 });
  console.log(JSON.stringify({ run, port, organization: org, project: project.id, environments: envs, writer: writer.id, outsider: outsider.id, envFile }));
}

const [cmd, ...rest] = process.argv.slice(2);
try {
  if (cmd === "up") await up(...rest);
  else if (cmd === "down") down();
  else throw new Error("usage: proof-secret-store.mjs up <vaultix-image> <env-file> [host] | down");
} catch (e) {
  console.error(e.message);
  process.exitCode = 1;
}
