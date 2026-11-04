import { useState } from "react";
import { Pencil, Plus, RefreshCw, Trash2 } from "lucide-react";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { Card, CardHeader } from "../../../lib/design/components/card/card";
import { DataTable, type ColumnDef } from "../../../lib/design/components/data-table/data-table";
import { Drawer } from "../../../lib/design/components/drawer/drawer";
import { EmptyState } from "../../../lib/design/components/empty-state/empty-state";
import { TextField } from "../../../lib/design/components/field/text-field";
import { Select } from "../../../lib/design/components/select/select";
import { StatusDot } from "../../../lib/design/components/status-dot/status-dot";
import { useDeleteNetwork, useNetworkConnections, useNetworkResources, useSaveNetwork, useSyncNetwork, useTestNetwork, type NetworkConnection, type NetworkConnectionInput, type NetworkProvider, type NetworkResource } from "../../../lib/hooks/use-infra";
import { useInfraCan } from "../../../lib/hooks/use-infra";

const providerLabels: Record<NetworkProvider, string> = {
  cloudflare_tunnel: "Cloudflare Tunnel",
  tailscale: "Tailscale",
  custom: "Custom inventory API",
};

const resourceColumns: ColumnDef<NetworkResource>[] = [
  { id: "name", label: "Resource", isRowHeader: true, render: (r) => <span className="font-medium text-fg">{r.name}</span> },
  { id: "kind", label: "Type", render: (r) => r.kind },
  { id: "state", label: "State", render: (r) => r.state || "—" },
  { id: "address", label: "Address", render: (r) => <span className="font-mono text-xs">{r.address || "—"}</span> },
  { id: "observed", label: "Observed", render: (r) => new Date(r.observed_at).toLocaleString() },
];

function configFor(c?: NetworkConnection): Record<string, string> {
  return c?.config ?? {};
}

function IntegrationCard({ item, canWrite, onEdit }: { item: NetworkConnection; canWrite: boolean; onEdit: () => void }) {
  const q = useNetworkResources(item.id, true);
  const sync = useSyncNetwork();
  const remove = useDeleteNetwork();
  const [note, setNote] = useState("");
  const statusTone = item.status === "healthy" ? "success" : item.status === "degraded" ? "danger" : "warning";
  const detail = item.config.account_id ?? item.config.tailnet ?? item.config.inventory_url ?? "";
  return (
    <Card elevation={2} className="overflow-hidden">
      <CardHeader title={item.label} description={`${providerLabels[item.provider]}${detail ? ` · ${detail}` : ""}`} />
      <div className="flex flex-wrap items-center gap-2 px-5 pb-4">
        <Badge tone={statusTone}><StatusDot health={item.status === "healthy" ? "up" : "unknown"} /> {item.status}</Badge>
        <Badge>{q.data?.items.length ?? 0} observed resources</Badge>
        {item.has_credential && <Badge tone="neutral">Credential sealed</Badge>}
        {item.last_checked && <span className="text-xs text-fg-subtle">Checked {new Date(item.last_checked).toLocaleString()}</span>}
      </div>
      {item.last_error && <p role="status" className="mx-5 mb-4 rounded-md border border-danger/30 bg-danger/5 px-3 py-2 text-xs text-danger">{item.last_error}</p>}
      <div className="px-5 pb-5">
        <DataTable label={`${item.label} resources`} columns={resourceColumns} rows={q.data?.items ?? []}
          empty={<EmptyState title={q.isLoading ? "Loading inventory" : "No inventory captured"} description={q.error?.message ?? "Refresh this integration to read its current resources."} />} />
        <div className="mt-4 flex flex-wrap items-end gap-2 border-t border-line pt-4">
          <div className="min-w-48 flex-1"><TextField label="Change note" value={note} onChange={setNote} isDisabled={!canWrite} /></div>
          {canWrite && <>
            <Button variant="secondary" size="sm" tooltip="Read current resources from the configured provider" isDisabled={sync.isPending}
              onPress={() => sync.mutate(item.id)}><RefreshCw className="size-4" /> {sync.isPending ? "Refreshing" : "Refresh inventory"}</Button>
            <Button variant="ghost" size="sm" tooltip="Edit this connection" onPress={onEdit}><Pencil className="size-4" /> Edit</Button>
            <Button variant="danger" size="sm" tooltip="Remove this connection and its local inventory" isDisabled={note.trim().length < 5 || remove.isPending}
              onPress={() => remove.mutate({ id: item.id, note })}><Trash2 className="size-4" /> Remove</Button>
          </>}
          {sync.isError && <p role="alert" className="w-full text-xs text-danger">{sync.error.message}</p>}
        </div>
      </div>
    </Card>
  );
}

function ConnectionForm({ initial, onClose }: { initial?: NetworkConnection; onClose: () => void }) {
  const [label, setLabel] = useState(initial?.label ?? "");
  const [provider, setProvider] = useState<NetworkProvider>(initial?.provider ?? "cloudflare_tunnel");
  const [config, setConfig] = useState(configFor(initial));
  const [credential, setCredential] = useState("");
  const [syncInterval, setSyncInterval] = useState(String(initial?.sync_interval_seconds ?? 300));
  const [note, setNote] = useState("");
  const test = useTestNetwork();
  const save = useSaveNetwork();
  const put = (key: string) => (value: string) => setConfig((old) => ({ ...old, [key]: value }));
  const testable = label.trim().length > 0 && (provider === "custom" ? !!config.inventory_url : !!credential && (provider === "cloudflare_tunnel" ? !!config.account_id : !!config.tailnet && !!config.client_id));
  const payload: NetworkConnectionInput = { label: label.trim(), provider, config, sync_interval_seconds: Number(syncInterval), ...(credential ? { credential } : {}) };
  const testNow = () => test.mutate(payload);
  const saveNow = () => save.mutate({ ...payload, ...(initial ? { id: initial.id } : {}), note }, { onSuccess: onClose });
  return (
    <Drawer isOpen onOpenChange={(open) => !open && onClose()} title={initial ? "Edit network integration" : "Add network integration"}
      subtitle="Configuration and observed resources stay in this Shippin installation. Provider credentials are sealed before storage."
      footer={<>
        <div className="mr-auto"><TextField label="Change note" value={note} onChange={setNote} /></div>
        <Button variant="ghost" tooltip="Cancel" onPress={onClose}>Cancel</Button>
        <Button variant="secondary" tooltip="Test provider access and preview its inventory" isDisabled={!testable || test.isPending}
          onPress={testNow}>{test.isPending ? "Testing" : "Test connection"}</Button>
        <Button variant="primary" tooltip="Save provider configuration" isDisabled={!label.trim() || note.trim().length < 5 || save.isPending || (initial?.provider !== provider && !credential)}
          onPress={saveNow}>{save.isPending ? "Saving" : "Save"}</Button>
      </>}>
      <div className="space-y-4 px-6 py-5">
        <TextField label="Name" value={label} onChange={setLabel} isRequired />
        <TextField label="Inventory refresh interval (seconds)" value={syncInterval} onChange={setSyncInterval} type="number" isRequired
          description="Automatic inventory refresh cadence. Choose 60 to 86400 seconds." />
        <Select label="Provider" options={Object.entries(providerLabels).map(([id, name]) => ({ id, label: name }))}
          value={provider} onChange={(id) => { setProvider(id as NetworkProvider); setConfig({}); }} isDisabled={!!initial} />
        {provider === "cloudflare_tunnel" && <>
          <TextField label="Cloudflare account ID" value={config.account_id ?? ""} onChange={put("account_id")} mono isRequired />
          <TextField label={initial?.has_credential ? "Replace API token (optional)" : "API token"} value={credential} onChange={setCredential} type="password" autoComplete="new-password" isRequired={!initial?.has_credential}
            description="Use an account scoped token with Cloudflare Tunnel Read. Shippin reads tunnel inventory and never stores the token in plaintext." />
        </>}
        {provider === "tailscale" && <>
          <TextField label="Tailnet" value={config.tailnet ?? ""} onChange={put("tailnet")} mono isRequired />
          <TextField label="OAuth client ID" value={config.client_id ?? ""} onChange={put("client_id")} mono isRequired />
          <TextField label={initial?.has_credential ? "Replace OAuth client secret (optional)" : "OAuth client secret"} value={credential} onChange={setCredential} type="password" autoComplete="new-password" isRequired={!initial?.has_credential}
            description="Create a Tailscale trust credential scoped to devices:core:read. Shippin only reads device inventory." />
        </>}
        {provider === "custom" && <>
          <TextField label="Inventory URL" value={config.inventory_url ?? ""} onChange={put("inventory_url")} mono isRequired
            description="HTTPS endpoint returning JSON shaped as { items: [{ id, kind, name, state?, address? }] }. Redirects are rejected; HTTP is allowed only on loopback." />
          <TextField label="Bearer token (optional)" value={credential} onChange={setCredential} type="password" autoComplete="new-password"
            description={initial?.has_credential && !credential ? "A credential is already sealed. Leave blank to keep it." : "Sent only to this inventory endpoint and sealed at rest."} />
        </>}
        {test.data?.ok && <p role="status" className="rounded-md border border-success/30 bg-success/5 px-3 py-2 text-sm text-success">Connection succeeded. {test.data.resource_count} resources found. Save to register it in Shippin.</p>}
        {test.data?.error && <p role="alert" className="rounded-md border border-danger/30 bg-danger/5 px-3 py-2 text-sm text-danger">{test.data.error}</p>}
        {test.isError && <p role="alert" className="text-sm text-danger">{test.error.message}</p>}
        {save.isError && <p role="alert" className="text-sm text-danger">{save.error.message}</p>}
      </div>
    </Drawer>
  );
}

/** Installation-wide connectors plus their last observed external resources. */
export function NetworkConnectionsTab() {
  const q = useNetworkConnections();
  const canWrite = useInfraCan()("infra.network.write");
  const [editing, setEditing] = useState<NetworkConnection | null | false>(false);
  return (
    <div className="space-y-4">
      <Card elevation={1}>
        <CardHeader title="Network integrations" description="Configure tunnels, private network inventory, and a custom inventory API. Resource rows show the last provider observation." />
        {canWrite && <div className="px-5 pb-5"><Button variant="primary" size="sm" tooltip="Add Cloudflare Tunnel, Tailscale, or a custom inventory provider" onPress={() => setEditing(null)}><Plus className="size-4" /> Add integration</Button></div>}
      </Card>
      {q.isPending && <Card className="p-5">Loading network integrations…</Card>}
      {q.isError && <Card className="p-5 text-sm text-danger">Could not load integrations: {q.error.message}</Card>}
      {q.data?.items.length === 0 && <Card><EmptyState title="No network integrations configured" description="Add a provider to capture tunnel or device resources in this installation." /></Card>}
      {q.data?.items.map((item) => <IntegrationCard key={item.id} item={item} canWrite={canWrite} onEdit={() => setEditing(item)} />)}
      {editing !== false && <ConnectionForm initial={editing || undefined} onClose={() => setEditing(false)} />}
    </div>
  );
}
