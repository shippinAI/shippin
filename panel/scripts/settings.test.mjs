import test from "node:test";
import assert from "node:assert/strict";
import { applySettings, loadSettings, SETTINGS_KEY, validateSettings } from "../src/features/configuration/settings.ts";

const base = {
  appName: "Shippin", environment: "local", apiBaseUrl: "http://localhost:8080/v1",
  docsUrl: "", defaultTenantId: "", appMode: "customer", capabilityHints: {},
  auth: { enabled: false, issuerUrl: "", clientId: "", audience: "", scopes: "openid", redirectUri: "", sessionExpiryWarningSeconds: 120 }
};
const settings = {
  appName: "Example team", apiBaseUrl: "https://api.example.com/v1", docsUrl: "",
  authEnabled: true, issuerUrl: "https://id.example.com", clientId: "client",
  audience: "", redirectUri: "https://console.example.com/callback"
};

test("accepts operator-owned HTTPS and loopback development addresses", () => {
  assert.equal(validateSettings(settings), true);
  assert.equal(validateSettings({ ...settings, apiBaseUrl: "http://127.0.0.1:8080/v1" }), true);
  const result = applySettings(base, settings);
  assert.equal(result.apiBaseUrl, settings.apiBaseUrl);
  assert.equal(result.auth.issuerUrl, settings.issuerUrl);
  assert.equal(result.appName, settings.appName);
  assert.equal(base.apiBaseUrl, "http://localhost:8080/v1");
});

test("rejects insecure external URLs, embedded credentials and incomplete OIDC", () => {
  for (const change of [
    { apiBaseUrl: "http://api.example.com/v1" },
    { apiBaseUrl: "https://user:password@api.example.com/v1" },
    { apiBaseUrl: "https://api.example.com/v1?token=secret" },
    { issuerUrl: "javascript:alert(1)" },
    { issuerUrl: "" },
    { clientId: "" },
    { appName: "" }
  ]) assert.equal(validateSettings({ ...settings, ...change }), false);
});

test("restores valid browser settings and ignores malformed stored values", () => {
  const storage = (value) => ({ getItem: (key) => key === SETTINGS_KEY ? value : null });
  assert.equal(loadSettings(storage(JSON.stringify(settings)), base).apiBaseUrl, settings.apiBaseUrl);
  assert.deepEqual(loadSettings(storage("{"), base), base);
  assert.deepEqual(loadSettings(storage(JSON.stringify({ ...settings, apiBaseUrl: "http://example.com" })), base), base);
  assert.deepEqual(loadSettings({ getItem: () => { throw new Error("storage disabled"); } }, base), base);
});
