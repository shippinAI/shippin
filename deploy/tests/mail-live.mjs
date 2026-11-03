// Live trigger of every e-mail notice against a THROWAWAY tenant (never a real one). The admin
// signs in for real (fresh, so break-glass is allowed) as ops-test-admin; the customer acts with
// the tenant owner's PAT. Mail delivery and DKIM are then checked on Gold (mail-verify.py).
//   ADMIN_PW_FILE, OWNER_PAT_FILE, TENANT_ID, EXPIRY (a configured duration, e.g. 1m), EDGE_IP
//   WS_NAME: also create (then delete) a workspace for the workspace-ready notice.
import { chromium } from "playwright";
import { readFileSync } from "fs";
if (!process.env.CONSOLE_HOST) throw new Error("set CONSOLE_HOST to the console under test");
const base = `https://${process.env.CONSOLE_HOST}`, edge = process.env.EDGE_IP ?? "192.0.2.252";
const T = process.env.TENANT_ID, EXP = process.env.EXPIRY ?? "1m", WSN = process.env.WS_NAME;
const pw = readFileSync(process.env.ADMIN_PW_FILE, "utf8").trim(), owner = readFileSync(process.env.OWNER_PAT_FILE, "utf8").trim();
const b = await chromium.launch({ args: [`--host-resolver-rules=MAP *.${process.env.CONSOLE_HOST.split(".").slice(1).join(".")} ${edge}`] });
const A = await (await b.newContext({ ignoreHTTPSErrors: true })).newPage();
const C = await (await b.newContext({ ignoreHTTPSErrors: true })).newPage();
let fails = 0;
const check = (ok, what) => { if (!ok) fails++; console.log(`${ok ? "  ok  " : "  FAIL"} ${what}`); };
const api = async (p, method, path, data) => {
  const r = await p.request.fetch(base + path, { method, data, headers: p === C ? { authorization: "Bearer " + owner } : {} });
  return [r.status(), await r.json().catch(() => ({}))];
};
const ask = async (why, duration = "1h") => {
  const [s, d] = await api(A, "POST", "/v1/operator/access-requests", { tenant_id: T, reason: `Mail live test: ${why}`, duration });
  check(s === 201, `request (${why}) -> ${s}`); return d.id;
};
const decide = async (id, op) => { const [s] = await api(C, "POST", `/v1/tenants/${T}/access/${id}/${op}`); check(s === 200, `owner ${op} -> ${s}`); };

await A.goto(`${base}/auth/login?reauth=1&rd=/admin/tenants`);
await A.locator("input[name=loginName], #loginName").fill("ops-test-admin"); await A.keyboard.press("Enter");
await A.locator("input[type=password]").fill(pw); await A.keyboard.press("Enter");
for (let i = 0; i < 20 && !A.url().startsWith(base); i++) {
  const skip = A.getByRole("button", { name: /skip/i });
  if (await skip.isVisible().catch(() => false)) await skip.click();
  await A.waitForTimeout(700);
}
await A.waitForURL(base + "/admin/tenants");

const r1 = await ask("deny path"); await decide(r1, "deny");
const r2 = await ask("approve then revoke"); await decide(r2, "approve"); await decide(r2, "revoke");
const r3 = await ask("expiry", EXP); await decide(r3, "approve");
let [s, d] = await api(A, "POST", "/v1/operator/break-glass", { tenant_id: T, category: "security_incident", justification: "Mail live test: break-glass notice must reach the owner now" });
check(s === 201, `break-glass (security incident) -> ${s}`); const bg = d.id;
[s, d] = await api(A, "POST", "/v1/operator/break-glass", { tenant_id: T, category: "legal_hold", justification: "Mail live test: legal hold, owner notice must wait", withhold_notice: true });
check(s === 201, `break-glass (legal hold, notice withheld until ${d.notice_at}) -> ${s}`); const hold = d.id;
for (const id of [bg, hold]) { [s] = await api(A, "POST", `/v1/operator/access-requests/${id}/end`); check(s === 200, `end break-glass -> ${s}`); }
if (WSN) {
  [s, d] = await api(C, "POST", `/v1/tenants/${T}/workspaces`, { name: WSN, template_id: "blank", size: "small" });
  check(s === 201 || s === 202, `owner creates workspace ${WSN} -> ${s}`); const ws = d.id;
  for (let i = 0; i < 90 && d.state !== "running" && d.state !== "error"; i++) { await A.waitForTimeout(5000); [s, d] = await api(C, "GET", `/v1/tenants/${T}/workspaces/${ws}`); }
  check(d.state === "running", `workspace running (${d.state})`);
  [s] = await api(C, "POST", `/v1/tenants/${T}/workspaces/${ws}/actions/delete`); check(s < 300, `workspace deleted -> ${s}`);
}
// The sweeper expires r3 once its duration has passed.
const until = Date.now() + 4 * 60e3;
do { await A.waitForTimeout(10e3); [s, d] = await api(C, "GET", `/v1/tenants/${T}/access`); }
while (Date.now() < until && d.items?.find((g) => g.id === r3)?.status !== "expired");
check(d.items?.find((g) => g.id === r3)?.status === "expired", "expiry grant expired");
console.log(JSON.stringify({ deny: r1, revoke: r2, expire: r3, breakglass: bg, legal_hold: hold }));
await b.close(); process.exit(fails ? 1 : 0);
