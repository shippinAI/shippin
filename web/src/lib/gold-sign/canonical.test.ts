import { b64url, canonical, challengeFor, fromB64url } from "./canonical";
import { bypassSummary, timeLeft } from "./bypass-model";

let failed = 0;
const eq = (name: string, got: unknown, want: unknown) => {
  if (JSON.stringify(got) !== JSON.stringify(want)) {
    failed++;
    console.error(`FAIL ${name}: got ${JSON.stringify(got)} want ${JSON.stringify(want)}`);
  }
};
const throws = (name: string, f: () => unknown) => {
  try {
    f();
    failed++;
    console.error(`FAIL ${name}: did not throw`);
  } catch {
    /* expected */
  }
};

const a64 = "a".repeat(64);
const p = { type: "approval" as const, ticket_id: "GC-1", ticket_sha256: a64, decision: "approve" as const, ts: 1 };
eq("canonical", canonical(p), `{"decision":"approve","ticket_id":"GC-1","ticket_sha256":"${a64}","ts":1,"type":"approval"}`);
// Same vector as internal/goldverify TestCanonicalVector (Go).
eq("challenge vector", b64url(await challengeFor(p)), "6mjg305JKH7E9sjXHlhfX3KZpCU4YfhpsartOgCavYA");
eq("revoke", canonical({ type: "revoke", cid: "cert-7", decision: "revoke", ts: 5 }), `{"cid":"cert-7","decision":"revoke","ts":5,"type":"revoke"}`);
throws("type mismatch", () => canonical({ ...p, type: "revoke" as "approval" }));
throws("quote in id", () => canonical({ ...p, ticket_id: 'x","decision":"approve' }));
throws("short sha", () => canonical({ ...p, ticket_sha256: "abc" }));
throws("bad decision", () => canonical({ ...p, decision: "approve-all" as "approve" }));
eq("b64 roundtrip", Array.from(fromB64url(b64url(new Uint8Array([0, 250, 251, 255])))), [0, 250, 251, 255]);

eq("time left h", timeLeft(1000, 1000 + 2 * 3600 + 5 * 60), "2h 5m");
eq("time left m", timeLeft(1000, 1000 + 90), "1m 30s");
eq("time left done", timeLeft(1000, 999), "expired");
eq("none", bypassSummary([], 0).label, "No bypass active");
eq("unknown", bypassSummary([], 0, false).tone, "warning");
eq("unknown label", bypassSummary([], 0, false).label, "Bypass status unknown (Gold not synced)");
eq("none tone", bypassSummary([], 0).tone, "neutral");
const c = { cid: "c1", holder_agent: "node", scope: ["gold:T1", "host:gold"], nbf: 0, exp: 3600, max_uses: 3, uses: 1, reason: "", revoke_sent: false };
eq("one", bypassSummary([c], 0).label, "Bypass active · node · gold:T1 +1 · 1h 0m left");
eq("one short", bypassSummary([c], 0).short, "Bypass node · gold:T1 +1 · 1h 0m");
eq("one tone", bypassSummary([c], 0).tone, "danger");
eq("two", bypassSummary([c, { ...c, cid: "c2", holder_agent: "codex" }], 0).label, "2 bypasses active · node, codex · 1h 0m left");

if (failed) {
  console.error(`${failed} failed`);
  process.exit(1);
}
console.log("gold-sign: all passed");
