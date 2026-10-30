import type { RuntimeConfig } from "../../shared/contracts";

export type ConnectionSettings = {
  appName: string;
  apiBaseUrl: string;
  docsUrl: string;
  authEnabled: boolean;
  issuerUrl: string;
  clientId: string;
  audience: string;
  redirectUri: string;
};

export const SETTINGS_KEY = "shippin-browser-connection-settings";

export function currentSettings(config: RuntimeConfig): ConnectionSettings {
  return {
    appName: config.appName,
    apiBaseUrl: config.apiBaseUrl,
    docsUrl: config.docsUrl,
    authEnabled: config.auth.enabled,
    issuerUrl: config.auth.issuerUrl,
    clientId: config.auth.clientId,
    audience: config.auth.audience,
    redirectUri: config.auth.redirectUri
  };
}

function validUrl(value: string, required: boolean): boolean {
  if (!value) return !required;
  try {
    const url = new URL(value);
    const loopback = ["localhost", "127.0.0.1", "[::1]"].includes(url.hostname);
    return (url.protocol === "https:" || (url.protocol === "http:" && loopback)) &&
      !url.username && !url.password && !url.search && !url.hash;
  } catch {
    return false;
  }
}

export function validateSettings(value: unknown): value is ConnectionSettings {
  if (!value || typeof value !== "object") return false;
  const s = value as Record<string, unknown>;
  const strings = ["appName", "apiBaseUrl", "docsUrl", "issuerUrl", "clientId", "audience", "redirectUri"];
  if (!strings.every((key) => typeof s[key] === "string") || typeof s.authEnabled !== "boolean") return false;
  const settings = s as ConnectionSettings;
  return settings.appName.trim().length > 0 && settings.appName.length <= 100 &&
    validUrl(settings.apiBaseUrl, true) && validUrl(settings.docsUrl, false) &&
    validUrl(settings.issuerUrl, settings.authEnabled) &&
    validUrl(settings.redirectUri, false) && (!settings.authEnabled || !!settings.clientId.trim());
}

export function applySettings(config: RuntimeConfig, settings: ConnectionSettings): RuntimeConfig {
  return {
    ...config,
    appName: settings.appName.trim(),
    apiBaseUrl: settings.apiBaseUrl.trim(),
    docsUrl: settings.docsUrl.trim(),
    auth: {
      ...config.auth,
      enabled: settings.authEnabled,
      issuerUrl: settings.issuerUrl.trim(),
      clientId: settings.clientId.trim(),
      audience: settings.audience.trim(),
      redirectUri: settings.redirectUri.trim()
    }
  };
}

export function loadSettings(storage: Storage, config: RuntimeConfig): RuntimeConfig {
  try {
    const raw = storage.getItem(SETTINGS_KEY);
    if (!raw) return config;
    const settings: unknown = JSON.parse(raw);
    return validateSettings(settings) ? applySettings(config, settings) : config;
  } catch {
    return config;
  }
}
