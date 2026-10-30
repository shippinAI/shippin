import type { RuntimeConfig } from "../../shared/contracts";

const PKCE_STATE_KEY = "shippin-oidc-state";
const PKCE_VERIFIER_KEY = "shippin-oidc-verifier";

function base64UrlEncode(input: ArrayBuffer): string {
  const bytes = new Uint8Array(input);
  let text = "";
  for (let i = 0; i < bytes.length; i += 1) {
    text += String.fromCharCode(bytes[i]);
  }
  return btoa(text).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}

function randomString(length: number): string {
  const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789";
  const output: string[] = [];
  const randomValues = new Uint32Array(length);
  crypto.getRandomValues(randomValues);
  for (let i = 0; i < length; i += 1) {
    output.push(alphabet[randomValues[i] % alphabet.length]);
  }
  return output.join("");
}

async function sha256(input: string): Promise<ArrayBuffer> {
  const encoded = new TextEncoder().encode(input);
  return crypto.subtle.digest("SHA-256", encoded);
}

function ensureAuthEnabled(config: RuntimeConfig): void {
  if (!config.auth.enabled) {
    throw new Error("OIDC auth is not enabled in runtime config.");
  }
  if (!config.auth.issuerUrl || !config.auth.clientId) {
    throw new Error("OIDC config is incomplete: issuerUrl and clientId are required.");
  }
}

async function discoverOidcEndpoints(config: RuntimeConfig): Promise<{ authorization: string; token: string }> {
  const issuer = new URL(config.auth.issuerUrl);
  const isSecure = (url: URL) => url.protocol === "https:" ||
    (url.protocol === "http:" && ["localhost", "127.0.0.1", "[::1]"].includes(url.hostname));
  if (!isSecure(issuer) || issuer.username || issuer.password || issuer.search || issuer.hash) {
    throw new Error("OIDC issuer must be an HTTPS URL without credentials, query or fragment.");
  }
  const issuerValue = issuer.href.replace(/\/$/, "");
  const response = await fetch(`${issuerValue}/.well-known/openid-configuration`, { cache: "no-store" });
  if (!response.ok) throw new Error(`OIDC discovery failed (${response.status}).`);
  const metadata: unknown = await response.json();
  if (!metadata || typeof metadata !== "object") throw new Error("OIDC discovery response is invalid.");
  const item = metadata as Record<string, unknown>;
  if (item.issuer !== issuerValue || typeof item.authorization_endpoint !== "string" || typeof item.token_endpoint !== "string") {
    throw new Error("OIDC discovery did not match the configured issuer or omitted an endpoint.");
  }
  const authUrl = new URL(item.authorization_endpoint);
  const tokenUrl = new URL(item.token_endpoint);
  if (![authUrl, tokenUrl].every((url) => isSecure(url) && !url.username && !url.password && !url.hash)) {
    throw new Error("OIDC discovery returned an insecure or invalid endpoint.");
  }
  return { authorization: authUrl.href, token: tokenUrl.href };
}

function resolveRedirectUri(config: RuntimeConfig): string {
  if (config.auth.redirectUri.trim()) {
    return config.auth.redirectUri.trim();
  }
  return `${window.location.origin}${window.location.pathname}`;
}

export async function startOidcLogin(config: RuntimeConfig): Promise<void> {
  ensureAuthEnabled(config);
  const endpoints = await discoverOidcEndpoints(config);
  const state = randomString(48);
  const verifier = randomString(96);
  const challenge = base64UrlEncode(await sha256(verifier));

  sessionStorage.setItem(PKCE_STATE_KEY, state);
  sessionStorage.setItem(PKCE_VERIFIER_KEY, verifier);

  const authorizeUrl = new URL(endpoints.authorization);
  authorizeUrl.searchParams.set("client_id", config.auth.clientId);
  authorizeUrl.searchParams.set("response_type", "code");
  authorizeUrl.searchParams.set("redirect_uri", resolveRedirectUri(config));
  authorizeUrl.searchParams.set("scope", config.auth.scopes || "openid profile email");
  authorizeUrl.searchParams.set("state", state);
  authorizeUrl.searchParams.set("code_challenge", challenge);
  authorizeUrl.searchParams.set("code_challenge_method", "S256");
  if (config.auth.audience.trim()) {
    authorizeUrl.searchParams.set("audience", config.auth.audience.trim());
  }

  window.location.assign(authorizeUrl.toString());
}

export async function completeOidcCodeFlowFromUrl(
  config: RuntimeConfig
): Promise<string | null> {
  ensureAuthEnabled(config);
  const currentUrl = new URL(window.location.href);
  const code = currentUrl.searchParams.get("code");
  const state = currentUrl.searchParams.get("state");
  if (!code) {
    return null;
  }

  const savedState = sessionStorage.getItem(PKCE_STATE_KEY);
  const savedVerifier = sessionStorage.getItem(PKCE_VERIFIER_KEY);
  if (!savedState || !savedVerifier) {
    throw new Error("Missing PKCE session state. Start login again.");
  }
  if (state !== savedState) {
    throw new Error("OIDC state mismatch. Start login again.");
  }

  const endpoints = await discoverOidcEndpoints(config);
  const tokenUrl = new URL(endpoints.token);
  const body = new URLSearchParams();
  body.set("grant_type", "authorization_code");
  body.set("code", code);
  body.set("redirect_uri", resolveRedirectUri(config));
  body.set("client_id", config.auth.clientId);
  body.set("code_verifier", savedVerifier);

  const response = await fetch(tokenUrl.toString(), {
    method: "POST",
    headers: { "Content-Type": "application/x-www-form-urlencoded" },
    body: body.toString()
  });
  if (!response.ok) {
    const responseBody = await response.text();
    throw new Error(
      `OIDC token exchange failed (${response.status}): ${responseBody || response.statusText}`
    );
  }

  const payload = (await response.json()) as { access_token?: string };
  if (!payload.access_token) {
    throw new Error("OIDC token response did not include access_token.");
  }

  currentUrl.searchParams.delete("code");
  currentUrl.searchParams.delete("state");
  currentUrl.searchParams.delete("session_state");
  window.history.replaceState({}, document.title, currentUrl.toString());
  sessionStorage.removeItem(PKCE_STATE_KEY);
  sessionStorage.removeItem(PKCE_VERIFIER_KEY);
  return payload.access_token;
}
