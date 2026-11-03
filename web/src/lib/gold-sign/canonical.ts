/** Signed payloads. Byte-identical to goldverify.Payload.Canonical (Go); see canonical.test.ts. */
export type TicketDecision = { type: "approval"; ticket_id: string; ticket_sha256: string; decision: "approve" | "deny"; ts: number };
export type RevokeDecision = { type: "revoke"; cid: string; decision: "revoke"; ts: number };
export type Decision = TicketDecision | RevokeDecision;

const ID = /^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$/;
const SHA = /^[0-9a-f]{64}$/;

/** JCS (sorted keys, no whitespace). Inputs are validated so no value needs escaping. */
export function canonical(p: Decision): string {
  if (!Number.isSafeInteger(p.ts) || p.ts <= 0) throw new Error("bad ts");
  if (p.decision === "revoke") {
    if (p.type !== "revoke" || !ID.test(p.cid)) throw new Error("bad cid");
    return `{"cid":"${p.cid}","decision":"revoke","ts":${p.ts},"type":"revoke"}`;
  }
  if (!ID.test(p.ticket_id) || !SHA.test(p.ticket_sha256)) throw new Error("bad ticket");
  if (p.decision !== "approve" && p.decision !== "deny") throw new Error("bad decision");
  if (p.type !== "approval") throw new Error("bad type");
  return `{"decision":"${p.decision}","ticket_id":"${p.ticket_id}","ticket_sha256":"${p.ticket_sha256}","ts":${p.ts},"type":"approval"}`;
}

/** WebAuthn challenge = sha256(canonical payload). */
export async function challengeFor(p: Decision): Promise<Uint8Array> {
  const d = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(canonical(p)));
  return new Uint8Array(d);
}

export function b64url(b: ArrayBuffer | Uint8Array): string {
  const u = b instanceof Uint8Array ? b : new Uint8Array(b);
  let s = "";
  for (const x of u) s += String.fromCharCode(x);
  return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

export function fromB64url(s: string): Uint8Array {
  const b = atob(s.replace(/-/g, "+").replace(/_/g, "/") + "===".slice((s.length + 3) % 4));
  return Uint8Array.from(b, (c) => c.charCodeAt(0));
}
