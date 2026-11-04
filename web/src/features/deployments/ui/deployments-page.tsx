import { Clock3, History, Pencil, Plus, Server, Trash2 } from "lucide-react";
import { useCallback, useMemo, useState } from "react";
import type { Json } from "../../../lib/api/catalog-types";
import { Badge } from "../../../lib/design/components/badge/badge";
import { Button } from "../../../lib/design/components/button/button";
import { IconButton } from "../../../lib/design/components/button/icon-button";
import { Card } from "../../../lib/design/components/card/card";
import { Drawer } from "../../../lib/design/components/drawer/drawer";
import { Select } from "../../../lib/design/components/select/select";
import { TextField } from "../../../lib/design/components/field/text-field";
import { useNavigate } from "../../../lib/router/router";
import { useTenant } from "../../../lib/hooks/use-tenant";
import {
  useCreateDeployment, useDeployableServices, useDeploymentEvents, useDeploymentTargets,
  useDeployments, useRequestDeploymentRemoval, useUpdateDeployment,
  type Deployment, type DeploymentService, type DeploymentSource,
} from "../../../lib/hooks/use-deployments";
import { PageHeader } from "../../../template/page-header";
import { QueryView } from "../../admin/ui/query-view";
import { ServiceConfigForm } from "./service-config-form";

const isObject = (value: unknown): value is Record<string, Json> => value !== null && typeof value === "object" && !Array.isArray(value);
const serviceSource = (source: DeploymentSource) => source.kind === "oci"
  ? source.reference
  : `${source.reference}@${source.commit}`;

export function DeploymentsPage() {
  const { tenant } = useTenant();
  const navigate = useNavigate();
  const tenantId = tenant?.tenant_id ?? "";
  const services = useDeployableServices(tenantId);
  const targets = useDeploymentTargets(tenantId);
  const canManage = !!targets.data?.can_request;
  const deployments = useDeployments(tenantId);
  const remove = useRequestDeploymentRemoval(tenantId);
  const [editing, setEditing] = useState<Deployment | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [historyId, setHistoryId] = useState("");
  const serviceById = useMemo(() => new Map((services.data?.items ?? []).map((item) => [item.id, item])), [services.data]);
  const targetById = useMemo(() => new Map((targets.data?.items ?? []).map((item) => [item.id, item])), [targets.data]);
  const requestableTargets = (targets.data?.items ?? []).filter((target) => target.state === "active" || target.state === "planned");
  const rows = deployments.data?.items ?? [];

  return <>
    <PageHeader eyebrow="Tenant services" title="Deployments"
      description="Configure services from the shared catalog on an Incus or Proxmox system assigned to this tenant."
      actions={canManage && <Button variant="primary" size="sm" isDisabled={!services.data?.items.length || !requestableTargets.length}
        tooltip={!requestableTargets.length ? "A configured assigned target is required" : "Request a service deployment"}
        onPress={() => setShowCreate(true)}><Plus className="size-4" /> Request deployment</Button>} />

    <Card className="mb-4 border-info/30 bg-info/5 p-4 text-sm text-fg-muted">
      Requests save a pinned service definition and desired configuration. This records intent only; it does not claim that a service is running. A configured AgentHub review and provider reconciler must complete the operation before observed state changes.
    </Card>

    {!canManage && <Card className="mb-4 p-4 text-sm text-fg-muted">A tenant owner or admin can add, edit, or request removal of deployments.</Card>}
    {canManage && (targets.isSuccess && !requestableTargets.length) && <Card className="mb-4 flex items-center justify-between gap-4 p-4">
      <p className="text-sm text-fg-muted">No configured Incus or Proxmox target is assigned to this tenant.</p>
      <Button variant="secondary" size="sm" tooltip="Open the tenant server registry" onPress={() => navigate(`/t/${tenantId}/servers`)}><Server className="size-4" /> Manage servers</Button>
    </Card>}

    <QueryView isPending={deployments.isPending || services.isPending || targets.isPending}
      error={deployments.error ?? services.error ?? targets.error}
      onRetry={() => { void deployments.refetch(); void services.refetch(); void targets.refetch(); }}>
      {() => rows.length === 0 ? <Card className="p-10 text-center">
        <h2 className="font-semibold text-fg">No deployments yet</h2>
        <p className="mx-auto mt-2 max-w-xl text-sm text-fg-muted">Active deployable services and targets appear here. Add a pinned service definition in the provider catalog, then assign an active Incus or Proxmox system to this tenant.</p>
        {canManage && <Button className="mt-4" variant="primary" isDisabled={!services.data?.items.length || !requestableTargets.length}
          tooltip="Record a deployment request from the active catalog" onPress={() => setShowCreate(true)}><Plus className="size-4" /> Request deployment</Button>}
      </Card> : <div className="grid gap-3">
        {rows.map((item) => {
          const service = serviceById.get(item.service_id);
          const target = targetById.get(item.target_system_id);
          return <Card key={item.id} className="p-4">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <h2 className="font-semibold text-fg">{service?.label ?? item.service_id}</h2>
                  <Badge tone={item.observed_state === "ready" ? "success" : item.observed_state === "failed" ? "danger" : "warning"}>{item.observed_state}</Badge>
                  <Badge>desired {item.desired_state}</Badge>
                  <Badge tone="info">{item.target_provider === "proxmox" ? "Shippin Prox" : "Incus"}</Badge>
                </div>
                <p className="mt-1 text-sm text-fg-muted">{target?.label ?? item.target_system_id} Â· generation {item.generation} Â· updated {new Date(item.updated_at).toLocaleString()}</p>
                <p className="mt-1 truncate font-mono text-xs text-fg-subtle" title={serviceSource(item.source)}>{serviceSource(item.source)}</p>
              </div>
              <div className="flex items-center gap-1">
                <Button variant="ghost" size="sm" tooltip="Show immutable generation and event history" onPress={() => setHistoryId(historyId === item.id ? "" : item.id)}><History className="size-4" /> History</Button>
                {canManage && item.desired_state === "present" && <IconButton icon={Pencil} label={`Edit ${service?.label ?? item.service_id}`} onPress={() => setEditing(item)} />}
                {canManage && item.desired_state === "present" && <IconButton icon={Trash2} label={`Request removal of ${service?.label ?? item.service_id}`}
                  onPress={() => {
                    if (window.confirm("Record desired absence for this service? This request does not delete a provider resource by itself.")) remove.mutate(item);
                  }} />}
              </div>
            </div>
            <details className="mt-3">
              <summary className="cursor-pointer text-xs text-fg-muted">Configuration</summary>
              <pre className="mt-2 max-h-48 overflow-auto rounded-md bg-canvas p-3 text-xs text-fg-muted">{JSON.stringify(item.config, null, 2)}</pre>
            </details>
            {historyId === item.id && <DeploymentHistory tenantId={tenantId} deployment={item} />}
          </Card>;
        })}
      </div>}
    </QueryView>

    {showCreate && <DeploymentEditor tenantId={tenantId} services={services.data?.items ?? []} targets={requestableTargets}
      onClose={() => setShowCreate(false)} />}
    {editing && <DeploymentEditor tenantId={tenantId} services={services.data?.items ?? []} targets={requestableTargets}
      deployment={editing} onClose={() => setEditing(null)} />}
  </>;
}

function DeploymentEditor({ tenantId, services, targets, deployment, onClose }: {
  tenantId: string; services: DeploymentService[]; targets: { id: string; label: string; provider: string; site: string; state: string }[];
  deployment?: Deployment; onClose: () => void;
}) {
  const [serviceId, setServiceId] = useState(deployment?.service_id ?? services[0]?.id ?? "");
  const service = services.find((item) => item.id === serviceId);
  const schema = deployment?.config_schema ?? service?.config_schema;
  const source = deployment?.source ?? service?.deployment_source;
  const [targetId, setTargetId] = useState(deployment?.target_system_id ?? targets[0]?.id ?? "");
  const [configText, setConfigText] = useState(() => JSON.stringify(deployment?.config ?? service?.default_config ?? {}, null, 2));
  const [schemaValid, setSchemaValid] = useState(true);
  const setSchemaValidity = useCallback((value: boolean) => setSchemaValid(value), []);
  const create = useCreateDeployment(tenantId);
  const update = useUpdateDeployment(tenantId);
  const config = (() => { try { return JSON.parse(configText) as unknown; } catch { return undefined; } })();
  const valid = isObject(config);
  const configObject = isObject(config) ? config : {};
  const busy = create.isPending || update.isPending;
  return <Drawer isOpen onOpenChange={(open) => !open && onClose()} title={deployment ? `Edit ${service?.label ?? deployment.service_id}` : "Request deployment"}
    subtitle="This records desired configuration and a pinned catalog revision; provider execution is not connected yet."
    footer={<>
      <div className="flex-1" />
      <Button variant="ghost" tooltip="Close without saving" onPress={onClose}>Cancel</Button>
      <Button variant="primary" tooltip={deployment ? "Append a generation using the current ETag" : "Record desired service configuration"} isDisabled={(!deployment && !service) || !valid || !schemaValid || busy || (!deployment && (!targetId || !targets.some((target) => target.id === targetId)))}
        onPress={() => {
          if (!isObject(config) || !schemaValid) return;
          if (deployment) update.mutate({ deployment, config }, { onSuccess: onClose });
          else if (service) create.mutate({ service_id: service.id, target_system_id: targetId, desired_state: "present", config }, { onSuccess: onClose });
        }}>{deployment ? "Save generation" : "Record request"}</Button>
    </>}>
    <div className="grid gap-4 px-6 py-5">
      {!deployment && <>
        <Select label="Service" value={serviceId} options={services.map((item) => ({ id: item.id, label: `${item.label} Â· ${item.kind}` }))}
          onChange={(id) => {
            setServiceId(id);
            const selected = services.find((item) => item.id === id);
            setConfigText(JSON.stringify(selected?.default_config ?? {}, null, 2));
          }} />
        <Select label="Target system" value={targetId} options={targets.map((target) => ({ id: target.id,
          label: `${target.provider === "proxmox" ? "Shippin Prox" : "Incus"} Â· ${target.label} Â· ${target.site}${target.state === "planned" ? " Â· connection unverified" : ""}` }))} onChange={setTargetId} />
      </>}
      {source && <div className="rounded-lg border border-line p-3">
        <div className="text-sm font-medium text-fg">Pinned source</div>
        <div className="mt-1 break-all font-mono text-xs text-fg-muted">{serviceSource(source)}</div>
        {service?.description && <div className="mt-2 text-xs text-fg-subtle">{service.description}</div>}
      </div>}
      {schema && valid
        ? <ServiceConfigForm key={deployment?.id ?? serviceId} schema={schema} value={configObject} onChange={(next) => setConfigText(JSON.stringify(next, null, 2))} onValidityChange={setSchemaValidity} />
        : <p className="text-sm text-fg-muted">{valid ? "This service has no editable schema. Use the advanced JSON editor." : "Enter a valid JSON object in the advanced editor."}</p>}
      <details className="rounded-lg border border-line p-3">
        <summary className="cursor-pointer text-sm font-medium text-fg">Advanced JSON editor</summary>
        <TextField label="Configuration (JSON)" multiline mono value={configText} onChange={setConfigText}
          description={valid ? "The server validates these values against the pinned service schema when saved." : "Enter a valid JSON object."} />
      </details>
      {schema && <details className="rounded-lg border border-line p-3">
        <summary className="cursor-pointer text-sm font-medium text-fg">Configuration schema and required fields</summary>
        <p className="mt-2 text-xs text-fg-muted">Secret defaults are hidden. Enter a Vaultix reference (vaultix://…) or a SOPS reference with a key pointer (sops://file#/path); never enter secret values.</p>
        <pre className="mt-2 max-h-64 overflow-auto rounded-md bg-canvas p-3 text-xs text-fg-muted">{JSON.stringify(schema, null, 2)}</pre>
      </details>}
      {deployment && <div className="flex items-center gap-2 text-xs text-fg-subtle"><Clock3 className="size-3.5" /> Saving appends generation {deployment.generation + 1}; previous generations remain in history.</div>}
    </div>
  </Drawer>;
}

function DeploymentHistory({ tenantId, deployment }: { tenantId: string; deployment: Deployment }) {
  const query = useDeploymentEvents(tenantId, deployment.id);
  if (query.isPending) return <p className="mt-3 text-xs text-fg-muted">Loading event history…</p>;
  if (query.error) return <p className="mt-3 text-xs text-danger">{query.error.message}</p>;
  return <ol className="mt-3 grid gap-2 border-l border-line pl-4">
    {(query.data?.items ?? []).map((event) => <li key={event.id} className="text-xs text-fg-muted">
      <span className="font-medium text-fg">{event.event_type}</span> Â· generation {event.generation} Â· {event.actor} Â· {new Date(event.recorded_at).toLocaleString()}
    </li>)}
  </ol>;
}
