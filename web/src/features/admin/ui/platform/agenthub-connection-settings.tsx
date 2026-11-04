import { useEffect, useState } from "react";
import { Button } from "../../../../lib/design/components/button/button";
import { TextField } from "../../../../lib/design/components/field/text-field";
import { useAgentHubConnection, useSetAgentHubConnection, useTestAgentHubConnection } from "../../../../lib/hooks/use-platform-settings";

export function AgentHubConnectionSettings({ canWrite }: { canWrite: boolean }) {
  const query = useAgentHubConnection();
  const save = useSetAgentHubConnection();
  const test = useTestAgentHubConnection();
  const [baseURL, setBaseURL] = useState("");
  const [tenantID, setTenantID] = useState("");
  const [credential, setCredential] = useState("");
  const [reason, setReason] = useState("");
  const [clearCredential, setClearCredential] = useState(false);
  useEffect(() => { if (query.data) setBaseURL(query.data.base_url); }, [query.data]);
  const dirty = !!baseURL.trim() && (baseURL.trim() !== query.data?.base_url || credential !== "" || clearCredential);
  const allowed = canWrite && dirty && reason.trim().length >= 5 && !save.isPending;
  return (
    <section className="space-y-3 border-t border-border px-4 py-5" aria-labelledby="agenthub-settings-title">
      <div>
        <h3 id="agenthub-settings-title" className="font-medium text-fg">AgentHub review connection</h3>
        <p className="text-sm text-fg-muted">AgentHub owns review policy and decisions. Shippin stores only the HTTPS endpoint and a sealed service credential. Provider changes stay disabled until the integration is verified.</p>
      </div>
      {query.isError && <p role="alert" className="text-sm text-danger">{query.error.message}</p>}
      {query.data && <p className="text-sm text-fg-muted">Credential: {query.data.has_credential ? "configured (write only)" : "not configured"}; integration: {query.data.configured ? "configured, not yet verified" : "incomplete"}.</p>}
      <TextField label="AgentHub HTTPS URL" value={baseURL} onChange={setBaseURL} isDisabled={!canWrite} placeholder="https://agenthub.example" />
      <TextField label="Replace service credential (write only)" type="password" value={credential} onChange={setCredential} isDisabled={!canWrite} autoComplete="new-password" />
      {query.data?.has_credential && <label className="flex items-center gap-2 text-sm text-fg"><input type="checkbox" checked={clearCredential} disabled={!canWrite} onChange={(e) => setClearCredential(e.target.checked)} />Clear stored credential</label>}
      <div className="flex items-end gap-2">
        <div className="flex-1"><TextField label="AgentHub tenant ID for read-only policy check" value={tenantID} onChange={(value) => { setTenantID(value); test.reset(); }} isDisabled={!canWrite} /></div>
        <Button size="sm" variant="secondary" isDisabled={!canWrite || !query.data?.configured || tenantID.trim().length < 3 || test.isPending} tooltip="Read policy metadata only; does not create a ticket or authorize an action" onPress={() => test.mutate({ tenant_id: tenantID.trim() })}>{test.isPending ? "Checking…" : "Test connection"}</Button>
      </div>
      {test.data && <p role={test.data.ok ? "status" : "alert"} className="text-sm text-fg-muted">{test.data.ok ? `Connected · policy ${test.data.policy_version} · ${test.data.policy_sha256}` : test.data.error}</p>}
      <div className="flex items-end gap-2">
        <div className="flex-1"><TextField label="Reason (audited)" value={reason} onChange={setReason} isDisabled={!canWrite} placeholder="Why this changes" /></div>
        <Button size="sm" variant="primary" isDisabled={!allowed} tooltip={allowed ? "Save the audited AgentHub connection settings" : "Enter a URL, provide a reason, and make a change"} onPress={() => save.mutate({ base_url: baseURL.trim(), ...(credential ? { credential } : {}), ...(clearCredential ? { clear_credential: true } : {}), reason }, { onSuccess: () => { setCredential(""); setReason(""); setClearCredential(false); } })}>Save connection</Button>
      </div>
    </section>
  );
}
