// Runs inside the Vaultix api container (tsx, Infisical's own SRP helper): creates the first
// instance admin with a normal (non-legacy) password login. Reads {email,password,port} JSON on
// stdin; prints only ids and status, never keys or tokens.
import { generateUserSrpKeys } from "@app/lib/crypto/srp";
import { crypto } from "@app/lib/crypto/cryptography";
import { initLogger } from "@app/lib/logger";

const main = async () => {
  let raw = "";
  for await (const c of process.stdin) raw += c;
  const { email, password, port } = JSON.parse(raw) as { email: string; password: string; port: string };
  await initLogger();
  await crypto.initialize(undefined as never, { ENCRYPTION_KEY: process.env.ENCRYPTION_KEY } as never);
  const k = await generateUserSrpKeys(email, password);
  const body = { email, password, firstName: "Vaultix", lastName: "Admin", protectedKey: k.protectedKey,
    protectedKeyIV: k.protectedKeyIV, protectedKeyTag: k.protectedKeyTag, publicKey: k.publicKey,
    encryptedPrivateKey: k.encryptedPrivateKey, encryptedPrivateKeyIV: k.encryptedPrivateKeyIV,
    encryptedPrivateKeyTag: k.encryptedPrivateKeyTag, salt: k.salt, verifier: k.verifier };
  const r = await fetch(`http://127.0.0.1:${port}/api/v1/admin/signup`,
    { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(body) });
  const j = (await r.json()) as { message?: string; user?: { id: string }; organization?: { id: string } };
  console.log(JSON.stringify({ status: r.status, message: j.message, user: j.user?.id, org: j.organization?.id }));
  process.exit(r.ok ? 0 : 1);
};
void main();
