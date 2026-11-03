import { apiGet, apiSend } from "./api/client";

type Challenge = { challenge: string; rp_id: string; rp_name: string };

const fromB64url = (s: string) => Uint8Array.from(atob(s.replace(/-/g, "+").replace(/_/g, "/").padEnd(Math.ceil(s.length / 4) * 4, "=")), (c) => c.charCodeAt(0));
const toB64 = (b: ArrayBuffer) => btoa(String.fromCharCode(...new Uint8Array(b)));
const challenge = () => apiSend<Challenge>("POST", "/v1/session/passkey/challenge");

/** What to tell someone when a passkey ceremony fails. A browser's own message says nothing useful. */
export function passkeyProblem(e: unknown) {
  const name = e instanceof DOMException ? e.name : "";
  if (name === "NotAllowedError" || name === "AbortError") return "That was cancelled, or took too long.";
  if (name === "SecurityError") return "Browsers refuse passkeys on a site with a certificate warning. Give this panel a real certificate, or sign in through an identity provider.";
  if (name === "NotSupportedError" || !window.PublicKeyCredential) return "This browser cannot use passkeys here.";
  return e instanceof Error ? e.message : "The passkey did not work.";
}

/** Adds a passkey for whoever is signed in. */
export async function addPasskey(user: { id: string; email: string; name: string }) {
  const c = await challenge();
  const cred = (await navigator.credentials.create({
    publicKey: {
      challenge: fromB64url(c.challenge),
      rp: { id: c.rp_id, name: c.rp_name },
      user: { id: new TextEncoder().encode(user.id), name: user.email, displayName: user.name || user.email },
      pubKeyCredParams: [{ type: "public-key", alg: -7 }],
      authenticatorSelection: { residentKey: "required", userVerification: "preferred" },
      attestation: "none",
      timeout: 60_000,
    },
  })) as PublicKeyCredential | null;
  if (!cred) throw new Error("No passkey was made.");
  const r = cred.response as AuthenticatorAttestationResponse;
  await apiSend("POST", "/v1/session/passkey/register", { challenge: c.challenge, client_data_json: toB64(r.clientDataJSON), authenticator_data: toB64(r.getAuthenticatorData()) });
}

/** Signs in with a passkey saved for this email. */
export async function signInWithPasskey(email: string) {
  const c = await challenge();
  const cred = (await navigator.credentials.get({ publicKey: { challenge: fromB64url(c.challenge), rpId: c.rp_id, userVerification: "preferred", timeout: 60_000 } })) as PublicKeyCredential | null;
  if (!cred) throw new Error("No passkey was chosen.");
  const r = cred.response as AuthenticatorAssertionResponse;
  await apiSend("POST", "/v1/session/passkey/login", { email, challenge: c.challenge, credential_id: toB64(cred.rawId), client_data_json: toB64(r.clientDataJSON), authenticator_data: toB64(r.authenticatorData), signature: toB64(r.signature) });
}

export type PasskeyRow = { id: string; created_at: string };
export const listPasskeys = () => apiGet<{ items: PasskeyRow[] }>("/v1/session/passkeys");
export const deletePasskey = (id: string) => apiSend("DELETE", `/v1/session/passkeys/${id}`);
