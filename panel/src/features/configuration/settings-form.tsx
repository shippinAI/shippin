import { useEffect, useState, type FormEvent } from "react";
import type { RuntimeConfig } from "../../shared/contracts";
import { currentSettings, validateSettings, type ConnectionSettings } from "./settings";

export function SettingsForm({ config, onSave }: {
  config: RuntimeConfig;
  onSave: (settings: ConnectionSettings) => void;
}) {
  const [draft, setDraft] = useState(() => currentSettings(config));
  const [error, setError] = useState("");
  useEffect(() => setDraft(currentSettings(config)), [config]);

  function change(key: keyof ConnectionSettings, value: string | boolean) {
    setDraft((current) => ({ ...current, [key]: value }));
  }
  function save(event: FormEvent) {
    event.preventDefault();
    if (!validateSettings(draft)) {
      setError("Enter a name and valid HTTPS URLs. HTTP is allowed only for localhost; URLs cannot contain credentials, query strings or fragments. OIDC also requires an issuer and client ID.");
      return;
    }
    setError("");
    onSave(draft);
  }
  return (
    <section className="panel">
      <h3>Connection settings</h3>
      <p className="muted">Applies only to this browser tab. Saving signs out the current session. Shared installation settings are not implemented yet.</p>
      <form onSubmit={save}>
        <div className="field-grid">
          <label>Display name<input value={draft.appName} onChange={(e) => change("appName", e.target.value)} /></label>
          <label>API base URL<input value={draft.apiBaseUrl} onChange={(e) => change("apiBaseUrl", e.target.value)} /></label>
          <label>API documentation URL (optional)<input value={draft.docsUrl} onChange={(e) => change("docsUrl", e.target.value)} /></label>
          <label>OIDC issuer URL<input value={draft.issuerUrl} onChange={(e) => change("issuerUrl", e.target.value)} /></label>
          <label>OIDC client ID<input value={draft.clientId} onChange={(e) => change("clientId", e.target.value)} /></label>
          <label>OIDC audience (optional)<input value={draft.audience} onChange={(e) => change("audience", e.target.value)} /></label>
          <label>OIDC redirect URL (optional)<input value={draft.redirectUri} onChange={(e) => change("redirectUri", e.target.value)} /></label>
          <label>Enable OIDC<input type="checkbox" checked={draft.authEnabled} onChange={(e) => change("authEnabled", e.target.checked)} /></label>
        </div>
        {error ? <p className="error-banner" role="alert">{error}</p> : null}
        <div className="action-row"><button type="submit">Save connection settings</button></div>
      </form>
    </section>
  );
}
