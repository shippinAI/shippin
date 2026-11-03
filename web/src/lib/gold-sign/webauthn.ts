import type { CredRef, RegBegin } from "../api/gold-sign-types";
import { b64url, challengeFor, fromB64url, type Decision } from "./canonical";

/** Envelope relayed to Gold. Public data only: no key ever leaves the authenticator. */
export type Envelope = {
  payload: Decision;
  kid: string;
  credential_id: string;
  authenticator_data: string;
  client_data_json: string;
  signature: string;
};

/** One tap: navigator.credentials.get with challenge = sha256(canonical payload), UV required. */
export async function signDecision(p: Decision, rpId: string, keys: CredRef[]): Promise<Envelope> {
  const cred = (await navigator.credentials.get({
    publicKey: {
      challenge: (await challengeFor(p)) as BufferSource,
      rpId,
      userVerification: "required",
      timeout: 60_000,
      allowCredentials: keys.map((k) => ({ type: "public-key" as const, id: fromB64url(k.credential_id) as BufferSource })),
    },
  })) as PublicKeyCredential | null;
  if (!cred) throw new Error("No passkey response");
  const r = cred.response as AuthenticatorAssertionResponse;
  const id = b64url(cred.rawId);
  return {
    payload: p,
    kid: keys.find((k) => k.credential_id === id)?.kid ?? "",
    credential_id: id,
    authenticator_data: b64url(r.authenticatorData),
    client_data_json: b64url(r.clientDataJSON),
    signature: b64url(r.signature),
  };
}

/** Passkey registration. The server decides who may register; displayName is the configured approver. */
export async function createPasskey(b: RegBegin, label: string) {
  const cred = (await navigator.credentials.create({
    publicKey: {
      challenge: fromB64url(b.challenge) as BufferSource,
      rp: { id: b.rp_id, name: "Gold approvals" },
      user: { id: new TextEncoder().encode(b.user_id), name: b.user_name, displayName: b.user_name },
      pubKeyCredParams: [{ type: "public-key", alg: -7 }, { type: "public-key", alg: -8 }],
      authenticatorSelection: { residentKey: "required", userVerification: "required" },
      excludeCredentials: b.exclude.map((k) => ({ type: "public-key" as const, id: fromB64url(k.credential_id) as BufferSource })),
      attestation: "none",
    },
  })) as PublicKeyCredential | null;
  if (!cred) throw new Error("No passkey created");
  const r = cred.response as AuthenticatorAttestationResponse;
  const spki = r.getPublicKey();
  if (!spki) throw new Error("This browser cannot export the public key");
  return { label, credential_id: b64url(cred.rawId), spki: b64url(spki), alg: r.getPublicKeyAlgorithm(), client_data_json: b64url(r.clientDataJSON) };
}
