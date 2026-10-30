import { FormEvent, useEffect, useMemo, useRef, useState } from "react";
import { executeApiRequest, fetchMe } from "../features/workbench/api";
import { InventoryPanel } from "../features/infrastructure/inventory-panel";
import { ConnectionPanel } from "../features/workbench/connection-panel";
import { OperationPanel } from "../features/workbench/operation-panel";
import { ResultPanel } from "../features/workbench/result-panel";
import { getCapabilityFromOperation, getPathParameters, hasOperatorRole, isCapabilityAllowed, isOperationAllowedForMode, resolveOperationPath } from "../features/workbench/operations";
import { completeOidcCodeFlowFromUrl, startOidcLogin } from "../features/auth/oidc";
import { SettingsForm } from "../features/configuration/settings-form";
import { applySettings, loadSettings, SETTINGS_KEY, type ConnectionSettings } from "../features/configuration/settings";
import { loadRuntimeConfig } from "../shared/runtime-config";
import openApiIndex from "../shared/generated/openapi-index.json";
import { CONSOLE_ROUTES } from "./routes";
import type {
  AppMode,
  ApiExecutionResult,
  MeIdentity,
  OpenApiIndex,
  OpenApiOperation,
  RuntimeConfig
} from "../shared/contracts";

const API_TOKEN_STORAGE_KEY = "shippin-control-panel-api-token";
const APP_MODE_STORAGE_KEY = "shippin-control-panel-app-mode";
const ACTIVE_ROUTE_STORAGE_KEY = "shippin-control-panel-active-route";

function decodeJwtPayload(token: string): Record<string, unknown> | null {
  const parts = token.split(".");
  if (parts.length < 2) {
    return null;
  }
  const payload = parts[1].replace(/-/g, "+").replace(/_/g, "/");
  const padding = payload.length % 4;
  const paddedPayload = `${payload}${padding > 0 ? "=".repeat(4 - padding) : ""}`;
  try {
    return JSON.parse(atob(paddedPayload)) as Record<string, unknown>;
  } catch {
    return null;
  }
}

function getTokenExpiryEpochMs(token: string): number | null {
  const payload = decodeJwtPayload(token);
  if (!payload || typeof payload.exp !== "number") {
    return null;
  }
  return payload.exp * 1000;
}

export default function App() {
  const apiIndex = openApiIndex as OpenApiIndex;
  const [config, setConfig] = useState<RuntimeConfig | null>(null);
  const [configError, setConfigError] = useState<string>("");
  const [token, setToken] = useState<string>("");
  const [appMode, setAppMode] = useState<AppMode>(
    () => (localStorage.getItem(APP_MODE_STORAGE_KEY) as AppMode) ?? "customer"
  );
  const [activeRouteId, setActiveRouteId] = useState<string>(
    () => localStorage.getItem(ACTIVE_ROUTE_STORAGE_KEY) ?? ""
  );
  const [selectedOperationId, setSelectedOperationId] = useState<string>("");
  const [tenantId, setTenantId] = useState<string>("");
  const [pathParamValues, setPathParamValues] = useState<Record<string, string>>({});
  const [requestBody, setRequestBody] = useState<string>("");
  const [result, setResult] = useState<ApiExecutionResult | null>(null);
  const [requestError, setRequestError] = useState<string>("");
  const [isSubmitting, setIsSubmitting] = useState<boolean>(false);
  const [identity, setIdentity] = useState<MeIdentity | null>(null);
  const [identityError, setIdentityError] = useState<string>("");
  const [isLoadingIdentity, setIsLoadingIdentity] = useState<boolean>(false);
  const identityRequestVersion = useRef(0);
  const operationRequestVersion = useRef(0);
  const operationController = useRef<AbortController | null>(null);
  const [authMessage, setAuthMessage] = useState<string>("");
  const [tokenExpiryEpochMs, setTokenExpiryEpochMs] = useState<number | null>(null);
  const [nowEpochMs, setNowEpochMs] = useState<number>(() => Date.now());

  function clearWorkbenchSession() {
    operationRequestVersion.current += 1;
    operationController.current?.abort();
    operationController.current = null;
    setIsSubmitting(false);
    setResult(null);
    setRequestError("");
    setRequestBody("");
    setPathParamValues({});
    setTenantId("");
  }

  useEffect(() => () => {
    operationRequestVersion.current += 1;
    operationController.current?.abort();
  }, []);

  useEffect(() => {
    // Remove credentials left by older workbench versions; never persist tokens.
    localStorage.removeItem(API_TOKEN_STORAGE_KEY);
  }, []);

  useEffect(() => {
    localStorage.setItem(APP_MODE_STORAGE_KEY, appMode);
  }, [appMode]);

  useEffect(() => {
    localStorage.setItem(ACTIVE_ROUTE_STORAGE_KEY, activeRouteId);
  }, [activeRouteId]);

  useEffect(() => {
    loadRuntimeConfig()
      .then(async (loaded) => {
        const activeConfig = loadSettings(sessionStorage, loaded.config);
        setConfig(activeConfig);
        setConfigError(loaded.warning);
        setAppMode(
          (localStorage.getItem(APP_MODE_STORAGE_KEY) as AppMode) ?? loaded.config.appMode
        );
        if (loaded.config.defaultTenantId) {
          setTenantId(loaded.config.defaultTenantId);
        }
        if (activeConfig.auth.enabled) {
          try {
            const accessToken = await completeOidcCodeFlowFromUrl(activeConfig);
            if (accessToken) {
              setToken(accessToken);
              setAuthMessage("OIDC login completed. Access token loaded.");
            }
          } catch (error) {
            const message = error instanceof Error ? error.message : "Unknown OIDC callback error";
            setIdentityError(message);
          }
        }
      })
      .catch((error) => {
        const message = error instanceof Error ? error.message : "Unknown config load error";
        setConfigError(`Could not load runtime config: ${message}`);
      });
  }, []);

  useEffect(() => {
    setTokenExpiryEpochMs(token.trim() ? getTokenExpiryEpochMs(token.trim()) : null);
  }, [token]);

  useEffect(() => {
    const timer = window.setInterval(() => setNowEpochMs(Date.now()), 15000);
    return () => window.clearInterval(timer);
  }, []);

  useEffect(() => {
    if (!token.trim()) {
      return;
    }
    if (!tokenExpiryEpochMs) {
      return;
    }
    if (nowEpochMs >= tokenExpiryEpochMs) {
      identityRequestVersion.current += 1;
      clearWorkbenchSession();
      setToken("");
      setIdentity(null);
      setIdentityError("Session expired. Sign in again.");
      setAuthMessage("");
    }
  }, [nowEpochMs, tokenExpiryEpochMs, token]);

  async function refreshIdentity() {
    if (!config || !token.trim()) {
      setIdentity(null);
      setIdentityError("Provide a bearer token to load /me identity.");
      return;
    }
    setIsLoadingIdentity(true);
    setIdentityError("");
    const requestVersion = ++identityRequestVersion.current;
    try {
      const me = await fetchMe(config.apiBaseUrl, token);
      if (requestVersion !== identityRequestVersion.current) return;
      setIdentity(me);
      if (!tenantId && me.tenants.length > 0) {
        setTenantId(me.tenants[0].tenant_id);
      }
    } catch (error) {
      if (requestVersion !== identityRequestVersion.current) return;
      const message = error instanceof Error ? error.message : "Unknown /me error";
      setIdentity(null);
      setIdentityError(message);
    } finally {
      if (requestVersion === identityRequestVersion.current) setIsLoadingIdentity(false);
    }
  }

  useEffect(() => {
    if (!config || !token.trim()) {
      setIdentity(null);
      setIsLoadingIdentity(false);
      return;
    }
    refreshIdentity().catch(() => {
      // handled by refreshIdentity
    });
    return () => { identityRequestVersion.current += 1; };
  }, [config, token]);

  const availableRoutes = useMemo(
    () =>
      CONSOLE_ROUTES.filter((route) => {
        if (route.mode !== appMode) {
          return false;
        }
        if (appMode === "operator" && !hasOperatorRole(identity)) {
          return false;
        }
        return route.tags.some((routeTag) => apiIndex.tags.includes(routeTag));
      }),
    [apiIndex.tags, appMode, identity]
  );

  useEffect(() => {
    if (!availableRoutes.length) {
      setActiveRouteId("");
      return;
    }
    if (!availableRoutes.some((route) => route.id === activeRouteId)) {
      setActiveRouteId(availableRoutes[0].id);
    }
  }, [availableRoutes, activeRouteId]);

  const activeRoute = useMemo(
    () => availableRoutes.find((route) => route.id === activeRouteId) ?? null,
    [availableRoutes, activeRouteId]
  );

  const operationsForRoute = useMemo(() => {
    if (!activeRoute) {
      return [] as OpenApiOperation[];
    }
    return apiIndex.operations.filter((operation) => {
      const operationTag = operation.tags[0] ?? "";
      return (
        activeRoute.tags.includes(operationTag) &&
        isOperationAllowedForMode(operation, appMode, identity)
      );
    });
  }, [activeRoute, apiIndex.operations, appMode, identity]);

  useEffect(() => {
    if (!operationsForRoute.length) {
      setSelectedOperationId("");
      return;
    }
    if (!operationsForRoute.some((operation) => operation.operationId === selectedOperationId)) {
      setSelectedOperationId(operationsForRoute[0].operationId);
    }
  }, [operationsForRoute, selectedOperationId]);

  const selectedOperation = useMemo(
    () =>
      operationsForRoute.find(
        (operation) => operation.operationId === selectedOperationId
      ) ?? null,
    [operationsForRoute, selectedOperationId]
  );

  const pathParameters = selectedOperation ? getPathParameters(selectedOperation.path) : [];

  const operatorModeBlocked = appMode === "operator" && !hasOperatorRole(identity);

  const sessionWarningSeconds = config?.auth.sessionExpiryWarningSeconds ?? 120;
  const sessionSecondsRemaining = tokenExpiryEpochMs
    ? Math.floor((tokenExpiryEpochMs - nowEpochMs) / 1000)
    : null;
  const sessionExpiringSoon =
    typeof sessionSecondsRemaining === "number" &&
    sessionSecondsRemaining > 0 &&
    sessionSecondsRemaining <= sessionWarningSeconds;

  const requiredCapability = selectedOperation
    ? getCapabilityFromOperation(selectedOperation, config)
    : "";
  const capabilityBlocked = selectedOperation
    ? !isCapabilityAllowed(selectedOperation, config, identity)
    : false;
  const totalVisibleOps = apiIndex.operations.filter((operation) =>
    isOperationAllowedForMode(operation, appMode, identity)
  ).length;

  async function onExecute(event: FormEvent) {
    event.preventDefault();
    if (!config || !selectedOperation || isSubmitting) return;
    if (capabilityBlocked) {
      setRequestError(`Blocked by capability guard: ${requiredCapability}`);
      return;
    }

    let fullPath: string;
    try {
      fullPath = resolveOperationPath(selectedOperation.path, { ...pathParamValues, tenantId });
    } catch (error) {
      setResult(null);
      setRequestError(error instanceof Error ? error.message : "Invalid path parameters.");
      return;
    }

    const fullUrl = `${config.apiBaseUrl.replace(/\/$/, "")}${fullPath}`;
    const requestVersion = ++operationRequestVersion.current;
    const controller = new AbortController();
    operationController.current = controller;
    setResult(null);
    setRequestError("");
    setIsSubmitting(true);
    try {
      const executionResult = await executeApiRequest({
        method: selectedOperation.method, url: fullUrl, token, body: requestBody, signal: controller.signal
      });
      if (requestVersion === operationRequestVersion.current) setResult(executionResult);
    } catch (error) {
      if (requestVersion !== operationRequestVersion.current) return;
      const message = error instanceof Error ? error.message : "Unknown API execution error";
      setRequestError(message);
      setResult(null);
    } finally {
      if (requestVersion === operationRequestVersion.current) {
        operationController.current = null;
        setIsSubmitting(false);
      }
    }
  }

  async function onLoginClick() {
    if (!config) {
      return;
    }
    try {
      await startOidcLogin(config);
    } catch (error) {
      const message = error instanceof Error ? error.message : "Unknown OIDC login error";
      setIdentityError(message);
    }
  }

  function onSignOutClick() {
    clearWorkbenchSession();
    identityRequestVersion.current += 1;
    setIsLoadingIdentity(false);
    setToken("");
    setIdentity(null);
    setTokenExpiryEpochMs(null);
    setAuthMessage("Signed out locally.");
  }

  function onSaveSettings(settings: ConnectionSettings) {
    if (!config) return;
    try {
      sessionStorage.setItem(SETTINGS_KEY, JSON.stringify(settings));
    } catch {
      setConfigError("Could not save browser settings. Check your browser storage settings.");
      return;
    }
    // Do not send a credential issued for the previous endpoint to the new one.
    clearWorkbenchSession();
    identityRequestVersion.current += 1;
    setIsLoadingIdentity(false);
    setToken("");
    setIdentity(null);
    setIdentityError("");
    setTenantId("");
    setResult(null);
    sessionStorage.removeItem("shippin-oidc-state");
    sessionStorage.removeItem("shippin-oidc-verifier");
    setConfig(applySettings(config, settings));
    setConfigError("");
    setAuthMessage("Connection settings saved for this browser tab. Sign in again.");
  }

  return (
    <div className="app-shell">
      <aside className="sidebar">
        <h1>{config?.appName ?? "Shippin"}</h1>
        <p className="muted">Control Panel</p>
        <div className="divider" />
        <label>
          App mode
          <select value={appMode} onChange={(event) => setAppMode(event.target.value as AppMode)}>
            <option value="customer">Customer console</option>
            <option value="operator">Operator console</option>
          </select>
        </label>
        <div className="divider" />
        {availableRoutes.map((route) => {
          const operationCount = apiIndex.operations.filter((operation) => {
            const primaryTag = operation.tags[0] ?? "";
            return (
              route.tags.includes(primaryTag) &&
              isOperationAllowedForMode(operation, appMode, identity)
            );
          }).length;
          return (
            <button
              key={route.id}
              className={`tag-button ${activeRouteId === route.id ? "active" : ""}`}
              onClick={() => setActiveRouteId(route.id)}
              type="button"
            >
              <span>{route.label}</span>
              {route.id !== "op-infrastructure" ? <span className="pill">{operationCount}</span> : null}
            </button>
          );
        })}
      </aside>

      <main className="main-content">
        <header className="topbar">
          <div>
            <h2>{config?.appName ?? "Loading..."}</h2>
            <p className="muted">
              Environment: <strong>{config?.environment ?? "..."}</strong>
            </p>
          </div>
          <div className="topbar-links">
            {config?.docsUrl ? <a href={config.docsUrl} target="_blank" rel="noreferrer">API docs</a> : null}
            <span className="muted">{totalVisibleOps} operations visible</span>
          </div>
        </header>

        {configError ? <p className="error-banner">{configError}</p> : null}
        {operatorModeBlocked ? (
          <p className="error-banner">
            Operator mode requires one of: op_support, op_admin, op_superadmin.
          </p>
        ) : null}
        {sessionExpiringSoon ? (
          <p className="warning-banner">
            Session expires in {sessionSecondsRemaining}s. Use Sign in with OIDC to renew.
          </p>
        ) : null}

        {config ? <SettingsForm config={config} onSave={onSaveSettings} /> : null}

        <section className="panel">
          <h3>Identity and auth</h3>
          <div className="field-grid">
            <label className="full-width">
              Bearer token
              <textarea
                value={token}
                onChange={(event) => {
                  identityRequestVersion.current += 1;
                  clearWorkbenchSession();
                  setIdentity(null);
                  setIdentityError("");
                  setAuthMessage("");
                  setIsLoadingIdentity(false);
                  setToken(event.target.value);
                }}
                placeholder="Paste an access token"
                rows={2}
              />
            </label>
          </div>
          <div className="action-row">
            <button type="button" onClick={() => refreshIdentity()} disabled={isLoadingIdentity}>
              {isLoadingIdentity ? "Loading /me..." : "Sync identity (/me)"}
            </button>
            <button type="button" onClick={() => onLoginClick()} disabled={!config?.auth.enabled}>
              Sign in with OIDC
            </button>
            <button type="button" onClick={onSignOutClick}>
              Sign out
            </button>
          </div>
          {authMessage ? <p className="success-banner">{authMessage}</p> : null}
          {identityError ? <p className="error-banner">{identityError}</p> : null}
          {identity ? (
            <div className="identity-grid">
              <p>
                <strong>User:</strong> {identity.display_name || identity.user_id}
              </p>
              <p>
                <strong>Platform roles:</strong>{" "}
                {(identity.platform_roles ?? []).join(", ") || "none"}
              </p>
              <p>
                <strong>Tenants:</strong> {identity.tenants.length}
              </p>
            </div>
          ) : (
            <p className="muted">No identity loaded.</p>
          )}
        </section>

        <ConnectionPanel config={config} tenantId={tenantId} identity={identity} onTenantChange={setTenantId} />
        {activeRoute?.id === "op-infrastructure" && config && identity && token.trim() ? (
          <InventoryPanel apiBaseUrl={config.apiBaseUrl} token={token} />
        ) : <OperationPanel
          routeLabel={activeRoute?.label ?? "No visible"}
          operations={operationsForRoute}
          selectedOperation={selectedOperation}
          selectedOperationId={selectedOperationId}
          pathParameters={pathParameters}
          tenantId={tenantId}
          pathParamValues={pathParamValues}
          requestBody={requestBody}
          isSubmitting={isSubmitting}
          operatorModeBlocked={operatorModeBlocked}
          capabilityBlocked={capabilityBlocked}
          requiredCapability={requiredCapability}
          requestError={requestError}
          onSelectOperation={setSelectedOperationId}
          onTenantChange={setTenantId}
          onPathParameterChange={(key, value) => setPathParamValues((current) => ({ ...current, [key]: value }))}
          onRequestBodyChange={setRequestBody}
          onExecute={onExecute}
        />}
        {activeRoute?.id !== "op-infrastructure" ? <ResultPanel result={result} /> : null}
      </main>
    </div>
  );
}
