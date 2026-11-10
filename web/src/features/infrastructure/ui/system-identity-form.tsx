import { useEffect, useState } from "react";
import { Button } from "../../../lib/design/components/button/button";
import { Card } from "../../../lib/design/components/card/card";
import { TextField } from "../../../lib/design/components/field/text-field";
import { useSaveSystemIdentity, useSystemIdentity, useTestSystemIdentityMapping } from "../../../lib/hooks/use-infra";

export function SystemIdentityForm({ serverId }: { serverId: string }) {
  const identity = useSystemIdentity(serverId);
  const save = useSaveSystemIdentity(serverId);
  const check = useTestSystemIdentityMapping(serverId);
  const [aliases, setAliases] = useState("");
  const [tags, setTags] = useState("");
  const [label, setLabel] = useState("");
  const [tenantId, setTenantId] = useState("");
  const [machineId, setMachineId] = useState("");
  const [note, setNote] = useState("");
  const [checkNote, setCheckNote] = useState("");
  useEffect(() => {
    if (!identity.data) return;
    setLabel(identity.data.label);
    setAliases(identity.data.aliases.join("\n"));
    setTags(identity.data.tags.join("\n"));
    setTenantId(identity.data.agenthub_tenant_id ?? "");
    setMachineId(identity.data.agenthub_machine_id ?? "");
  }, [identity.data]);
  if (identity.isPending) return <Card className="mb-6 p-5"><p className="text-base text-fg-muted">Loading system identity…</p></Card>;
  if (identity.isError) return <Card className="mb-6 p-5"><p role="alert" className="text-base text-danger">{identity.error.message}</p></Card>;
  const split = (value: string) => value.split(/[\n,]/).map((part) => part.trim()).filter(Boolean);
  const savedMappingMatchesForm = tenantId.trim() === (identity.data.agenthub_tenant_id ?? "") && machineId.trim() === (identity.data.agenthub_machine_id ?? "");
  const lastCheck = identity.data.agenthub_mapping_check;
  return (
    <Card elevation={2} className="mb-6 p-5">
      <div className="mb-4">
        <h2 className="text-base font-semibold text-fg">System identity</h2>
        <p className="mt-1 text-sm text-fg-muted">
          Shippin system ID: <span className="font-mono">{identity.data.server_id}</span>
          {identity.data.hostname && <> · Hostname: <span className="font-medium text-fg">{identity.data.hostname}</span> ({identity.data.hostname_source}{identity.data.hostname_observed_at ? `, observed ${new Date(identity.data.hostname_observed_at).toLocaleString()}` : ""})</>}
          {identity.data.identity_updated_at && <> · Metadata updated by {identity.data.identity_updated_by} at {new Date(identity.data.identity_updated_at).toLocaleString()}</>}
        </p>
        <p className="mt-1 text-sm text-fg-muted">AgentHub IDs are explicit references. The check confirms machine and tenant mapping only; it does not authorize deployments.</p>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <TextField label="Display name / nickname" value={label} onChange={setLabel} />
        <TextField label="Aliases (one per line)" multiline value={aliases} onChange={setAliases} />
        <TextField label="Tags (one per line)" multiline value={tags} onChange={setTags} />
        <TextField label="AgentHub tenant ID" value={tenantId} onChange={setTenantId} />
        <TextField label="AgentHub machine ID" value={machineId} onChange={setMachineId} />
      </div>
      <div className="mt-3 flex flex-wrap items-end gap-3">
        <TextField label="Reason for identity change" value={note} onChange={setNote} />
        <Button size="sm" variant="primary" tooltip="Save display name, aliases, tags, and an explicit AgentHub mapping" isDisabled={save.isPending || note.trim().length < 5 || !label.trim() || (!!tenantId.trim() !== !!machineId.trim())}
          onPress={() => save.mutate({ label, aliases: split(aliases), tags: split(tags), agenthub_tenant_id: tenantId.trim(), agenthub_machine_id: machineId.trim(), note })}>
          Save identity
        </Button>
        {save.isError && <p role="alert" className="text-base text-danger">{save.error.message}</p>}
      </div>
      <div className="mt-4 flex flex-wrap items-end gap-3 border-t border-line pt-4">
        <TextField label="Reason for mapping check" value={checkNote} onChange={setCheckNote} />
        <Button size="sm" variant="secondary" tooltip="Check the saved AgentHub machine and tenant mapping without authorizing a deployment" isDisabled={check.isPending || !savedMappingMatchesForm || !tenantId.trim() || !machineId.trim() || checkNote.trim().length < 5}
          onPress={() => check.mutate(checkNote.trim())}>Check AgentHub mapping</Button>
        {check.isError && <p role="alert" className="text-base text-danger">{check.error.message}</p>}
      </div>
      {lastCheck && <div className="mt-3 rounded-md border border-line p-3 text-sm" aria-live="polite">
        <p className={lastCheck.outcome === "verified" && lastCheck.matches_current ? "text-success" : "text-danger"}>
          {lastCheck.outcome === "verified" && lastCheck.matches_current ? "Verified" : "Not verified or stale"}: {lastCheck.detail}
        </p>
        <p className="mt-1 text-fg-muted">Checked {new Date(lastCheck.checked_at).toLocaleString()} by {lastCheck.checked_by}. Policy version {lastCheck.policy_version ?? "—"}; SHA-256 {lastCheck.policy_sha256 ?? "—"}.</p>
      </div>}
    </Card>
  );
}
