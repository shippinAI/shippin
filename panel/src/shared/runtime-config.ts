import type { RuntimeConfig } from "./contracts";

const DEFAULT_RUNTIME_CONFIG: RuntimeConfig = {
  appName: "Shippin Control Panel",
  environment: "local",
  apiBaseUrl: "http://localhost:8080/v1",
  docsUrl: "",
  defaultTenantId: "",
  appMode: "customer",
  capabilityHints: {},
  auth: {
    enabled: false,
    issuerUrl: "",
    clientId: "",
    audience: "",
    scopes: "openid profile email",
    redirectUri: "",
    sessionExpiryWarningSeconds: 120
  }
};

export type RuntimeConfigLoadResult = {
  config: RuntimeConfig;
  warning: string;
};

export async function loadRuntimeConfig(): Promise<RuntimeConfigLoadResult> {
  const runtimeConfigPath = `${import.meta.env.BASE_URL}config/runtime-config.json`;
  try {
    const response = await fetch(runtimeConfigPath, {
      method: "GET",
      headers: { "Content-Type": "application/json" }
    });
    if (!response.ok) {
      return {
        config: DEFAULT_RUNTIME_CONFIG,
        warning: `Runtime config request failed (${response.status}). Using defaults.`
      };
    }
    const loaded = (await response.json()) as Partial<RuntimeConfig>;
    return {
      config: {
        appName: loaded.appName ?? DEFAULT_RUNTIME_CONFIG.appName,
        environment: loaded.environment ?? DEFAULT_RUNTIME_CONFIG.environment,
        apiBaseUrl: loaded.apiBaseUrl ?? DEFAULT_RUNTIME_CONFIG.apiBaseUrl,
        docsUrl: loaded.docsUrl ?? DEFAULT_RUNTIME_CONFIG.docsUrl,
        defaultTenantId:
          loaded.defaultTenantId ?? DEFAULT_RUNTIME_CONFIG.defaultTenantId,
        appMode:
          loaded.appMode === "operator" ? "operator" : DEFAULT_RUNTIME_CONFIG.appMode,
        capabilityHints:
          loaded.capabilityHints ?? DEFAULT_RUNTIME_CONFIG.capabilityHints,
        auth: {
          enabled:
            typeof loaded.auth?.enabled === "boolean"
              ? loaded.auth.enabled
              : DEFAULT_RUNTIME_CONFIG.auth.enabled,
          issuerUrl:
            loaded.auth?.issuerUrl ?? DEFAULT_RUNTIME_CONFIG.auth.issuerUrl,
          clientId:
            loaded.auth?.clientId ?? DEFAULT_RUNTIME_CONFIG.auth.clientId,
          audience:
            loaded.auth?.audience ?? DEFAULT_RUNTIME_CONFIG.auth.audience,
          scopes: loaded.auth?.scopes ?? DEFAULT_RUNTIME_CONFIG.auth.scopes,
          redirectUri:
            loaded.auth?.redirectUri ?? DEFAULT_RUNTIME_CONFIG.auth.redirectUri,
          sessionExpiryWarningSeconds:
            typeof loaded.auth?.sessionExpiryWarningSeconds === "number" &&
            loaded.auth.sessionExpiryWarningSeconds > 0
              ? loaded.auth.sessionExpiryWarningSeconds
              : DEFAULT_RUNTIME_CONFIG.auth.sessionExpiryWarningSeconds
        }
      },
      warning: ""
    };
  } catch (error) {
    const message = error instanceof Error ? error.message : "Unknown fetch error";
    return {
      config: DEFAULT_RUNTIME_CONFIG,
      warning: `Runtime config load failed (${message}). Using defaults.`
    };
  }
}
