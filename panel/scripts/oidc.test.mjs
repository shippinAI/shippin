import test from "node:test";
import assert from "node:assert/strict";
import { startOidcLogin } from "../src/features/auth/oidc.ts";

const config = { auth: { enabled: true, issuerUrl: "https://identity.example.com", clientId: "client", audience: "", scopes: "openid", redirectUri: "https://console.example.com/callback" } };

test("uses the configured provider's discovery endpoints", async () => {
  const originalFetch = globalThis.fetch;
  let requested = "", redirected = "";
  const values = new Map();
  globalThis.sessionStorage = { setItem: (key, value) => values.set(key, value) };
  globalThis.window = { location: { assign: (value) => { redirected = value; } } };
  globalThis.fetch = async (url) => {
    requested = String(url);
    return { ok: true, json: async () => ({ issuer: config.auth.issuerUrl, authorization_endpoint: "https://login.example.com/authorize", token_endpoint: "https://identity.example.com/token" }) };
  };
  try {
    await startOidcLogin(config);
    assert.equal(requested, "https://identity.example.com/.well-known/openid-configuration");
    assert.equal(new URL(redirected).origin + new URL(redirected).pathname, "https://login.example.com/authorize");
    assert.equal(new URL(redirected).searchParams.get("code_challenge_method"), "S256");
    assert.ok(values.get("shippin-oidc-state"));
  } finally { globalThis.fetch = originalFetch; delete globalThis.window; delete globalThis.sessionStorage; }
});

test("rejects discovery with the wrong issuer or an insecure endpoint", async () => {
  const originalFetch = globalThis.fetch;
  try {
    for (const metadata of [
      { issuer: "https://other.example.com", authorization_endpoint: "https://other.example.com/auth", token_endpoint: "https://other.example.com/token" },
      { issuer: config.auth.issuerUrl, authorization_endpoint: "http://identity.example.com/auth", token_endpoint: "https://identity.example.com/token" }
    ]) {
      globalThis.fetch = async () => ({ ok: true, json: async () => metadata });
      await assert.rejects(() => startOidcLogin(config));
    }
  } finally { globalThis.fetch = originalFetch; }
});

test("accepts an issuer whose registered identifier ends with a slash", async () => {
  const originalFetch = globalThis.fetch;
  const slashConfig = { auth: { ...config.auth, issuerUrl: "https://identity.example.com/" } };
  globalThis.sessionStorage = { setItem: () => {} };
  globalThis.window = { location: { assign: () => {} } };
  globalThis.fetch = async () => ({ ok: true, json: async () => ({
    issuer: slashConfig.auth.issuerUrl,
    authorization_endpoint: "https://identity.example.com/auth",
    token_endpoint: "https://identity.example.com/token"
  }) });
  try { await startOidcLogin(slashConfig); }
  finally { globalThis.fetch = originalFetch; delete globalThis.window; delete globalThis.sessionStorage; }
});
