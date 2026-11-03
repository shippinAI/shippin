// Live authz test on Gold: every throwaway user (staff-live-users.py) calls every operator route
// and must get 403 exactly where testdata/staff-matrix/*.txt says so. Placeholder ids never match
// a real object, so allowed write routes stop at 400/404 without changing anything.
// Needs mode enforce first (Team settings, or deploy/tests/staff-mode.sh enforce "reason").
// Usage: CREDS=creds.json MATRIX=internal/api/testdata/staff-matrix [ENFORCE=1] node staff-live.mjs (a dir of *.txt, read in name order)
import { chromium } from "playwright";
import { readFileSync, readdirSync, statSync } from "fs";
import { base, edgeArgs, login, call } from "./staff-login.mjs";

const users = JSON.parse(readFileSync(process.env.CREDS, "utf8"));
const readMatrix = (p) => statSync(p).isDirectory()
  ? readdirSync(p).filter((f) => f.endsWith(".txt")).sort().map((f) => readFileSync(`${p}/${f}`, "utf8")).join("\n")
  : readFileSync(p, "utf8");
const matrix = parseMatrix(readMatrix(process.env.MATRIX));
const fill = { tenantId: "00000000-0000-0000-0000-000000000000", workspaceId: "00000000-0000-0000-0000-000000000000",
  grantId: "00000000-0000-0000-0000-000000000000", requestId: "00000000-0000-0000-0000-000000000000",
  templateId: "live-probe-none", userId: "1", roleKey: "staff.live_probe_none", action: "stop", state: "deactivate",
  decision: "deny" };

function parseMatrix(txt) {
  const m = new Map(); let role = null;
  for (const line of txt.split("\n").map((l) => l.trim())) {
    if (!line || line.startsWith("#")) continue;
    if (line.startsWith("[")) { role = line.slice(1, -1); m.set(role, new Set()); continue; }
    m.get(role).add(line);
  }
  return m;
}

const allRoutes = [...new Set([...matrix.values()].flatMap((s) => [...s]).filter((r) => r !== "ALL"))];
const allowed = (role, route) => { const s = matrix.get(role); return !!s && (s.has("ALL") || s.has(route)); };
const concrete = (route) => route.replace(/\{(\w+)\}/g, (_, k) => fill[k] ?? "x");

const b = await chromium.launch({ args: edgeArgs });
let pass = 0, fail = 0;
const check = (ok, msg) => { ok ? pass++ : fail++; if (!ok) console.log("FAIL", msg); };
const order = [...users].sort((x, y) => (y.role === "staff.owner") - (x.role === "staff.owner"));
for (const u of order) {
  const ctx = await b.newContext();
  const p = await login(ctx, u);
  const me = await call(p, "GET", "/v1/me");
  const staff = me.body?.staff ?? {};
  check(me.status === 200, `${u.spec}: /v1/me ${me.status}`);
  check(!!staff.mfa === !!u.totp, `${u.spec}: /me mfa=${staff.mfa}`);
  if (u.role === "staff.owner" && process.env.ENFORCE && u.totp) {
    const cur = (await call(p, "GET", "/v1/operator/team")).body?.settings;
    const r = await call(p, "PUT", "/v1/operator/team/settings", { ...cur, mode: "enforce", reason: "Live staff-roles test: enforce mode with the IAM_OWNER fallback kept on." });
    check(r.status === 200, `owner: switch to enforce ${r.status}`);
  }
  const role = u.totp ? u.role : "none";
  for (const route of allRoutes) {
    const [m, path] = route.split(" ");
    const r = await call(p, m, concrete(path), m === "GET" ? undefined : {});
    const want = allowed(role, route);
    check(want ? r.status !== 403 && r.status < 500 : r.status === 403, `${u.spec}: ${route} → ${r.status} (want ${want ? "allowed" : "403"})`);
  }
  if (u.role === "staff.owner" && u.totp) {
    const self = await call(p, "PUT", `/v1/operator/team/members/${u.user_id}/roles`, { roles: ["staff.owner", "staff.auditor"], reason: "Live test: self-grant must be refused." });
    check(self.status === 403, `owner: self-grant → ${self.status} (want 403)`);
  }
  console.log(`${u.spec}: done (pass ${pass}, fail ${fail})`);
  await ctx.close();
}
await b.close();
console.log(`live: ${pass} passed, ${fail} failed against ${base}`);
process.exit(fail ? 1 : 0);
