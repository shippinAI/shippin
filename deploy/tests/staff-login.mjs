// Shippin ID login for live tests: password + TOTP through the hosted login UI.
// Credentials come from the staff-live-users.py file and are never printed.
import { createHmac } from "crypto";

if (!process.env.CONSOLE_URL) throw new Error("set CONSOLE_URL to the console under test");
export const base = process.env.CONSOLE_URL;
export const edgeArgs = process.env.EDGE_IP ? [`--host-resolver-rules=MAP *.${new URL(base).hostname.split(".").slice(1).join(".")} ${process.env.EDGE_IP}`] : [];

export function totp(secret, at = Date.now()) {
  const abc = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
  let bits = "";
  for (const ch of secret.replace(/=+$/, "")) bits += abc.indexOf(ch).toString(2).padStart(5, "0");
  const key = Buffer.from(bits.match(/.{8}/g).map((x) => parseInt(x, 2)));
  const msg = Buffer.alloc(8);
  msg.writeBigUInt64BE(BigInt(Math.floor(at / 30000)));
  const h = createHmac("sha1", key).update(msg).digest();
  const o = h[19] & 15;
  return String((h.readUInt32BE(o) & 0x7fffffff) % 1e6).padStart(6, "0");
}

// login signs the user in on a fresh page of ctx and returns that page (on the console).
export async function login(ctx, user) {
  const p = await ctx.newPage();
  await p.goto(base + "/login");
  await p.getByRole("button", { name: /Continue/ }).click();
  await p.locator("input[name=loginName]").fill(user.login);
  await p.keyboard.press("Enter");
  await p.locator("input[type=password]").fill(user.password);
  await p.keyboard.press("Enter");
  if (user.totp) {
    await p.locator("input[name=code]").fill(totp(user.totp));
    await p.keyboard.press("Enter");
  }
  await p.waitForURL((u) => u.origin === new URL(base).origin && !u.pathname.startsWith("/login"), { timeout: 20000 });
  return p;
}

// call runs a same-origin fetch inside the page (so the session cookie and CSRF rules apply).
export async function call(page, method, path, body) {
  return page.evaluate(async ([m, u, b]) => {
    const r = await fetch(u, { method: m, headers: b ? { "content-type": "application/json" } : {}, body: b });
    let j = null;
    try { j = await r.json(); } catch {}
    return { status: r.status, body: j };
  }, [method, path, body === undefined ? undefined : JSON.stringify(body)]);
}
