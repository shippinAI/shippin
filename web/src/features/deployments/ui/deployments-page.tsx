import { CheckCircle2, Clock3, History, Pencil, Plus, Server, Trash2 } from "lucide-react";
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
  useAuthorizeDeploymentReview, useDeploymentReview, useDeployments, usePrepareDeploymentReview, useRequestDeploymentRemoval, useUpdateDeployment,
  useDeploymentTargetStorage, useDeploymentTargetNetworks, type Deployment, type DeploymentService, type DeploymentSource, type DeploymentTargetConfig,
} from "../../../lib/hooks/use-deployments";
import { PageHeader } from "../../../template/page-header";
import { QueryView } from "../../admin/ui/query-view";
import { ServiceConfigForm } from "./service-config-form";
import {
  currentGenerationFailure, deploymentEventLabel, deploymentEventSummary, deploymentFailureMessage,
  deploymentObservedStateTone,
} from "../deployment-status";
import { requiresIncusTargetConfiguration, supportsRuntimeTarget } from "../runtime-targets";

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
      description="Configure services from the shared catalog on a compatible system assigned to this tenant."
      actions={canManage && <Button variant="primary" size="sm" isDisabled={!services.data?.items.length || !requestableTargets.length}
        tooltip={!requestableTargets.length ? "A configured assigned target is required" : "Request a service deployment"}
        onPress={() => setShowCreate(true)}><Plus className="size-4" /> Request deployment</Button>} />

    <Card className="mb-4 border-info/30 bg-info/5 p-4 text-base text-fg-muted">
      Requests save a pinned service definition and desired configuration. This records intent only; it does not claim that a service is running. A configured AgentHub review and provider reconciler must complete the operation before observed state changes.
    </Card>

    {!canManage && <Card className="mb-4 p-4 text-base text-fg-muted">A tenant owner or admin can add, edit, or request removal of deployments.</Card>}
    {canManage && (targets.isSuccess && !requestableTargets.length) && <Card className="mb-4 flex items-center justify-between gap-4 p-4">
      <p className="text-base text-fg-muted">No compatible target is assigned to this tenant.</p>
      <Button variant="secondary" size="sm" tooltip="Open the tenant server registry" onPress={() => navigate(`/t/${tenantId}/servers`)}><Server className="size-4" /> Manage servers</Button>
    </Card>}

    <QueryView isPending={deployments.isPending || services.isPending || targets.isPending}
      error={deployments.error ?? services.error ?? targets.error}
      onRetry={() => { void deployments.refetch(); void services.refetch(); void targets.refetch(); }}>
      {() => rows.length === 0 ? <Card className="p-10 text-center">
        <h2 className="font-semibold text-fg">No deployments yet</h2>
        <p className="mx-auto mt-2 max-w-xl text-base text-fg-muted">Active deployable services and compatible assigned targets appear here. Add a pinned service definition in the provider catalog, then assign a supported system to this tenant.</p>
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
                  <Badge tone={deploymentObservedStateTone(item.observed_state)}>{item.observed_state}</Badge>
                  <Badge>desired {item.desired_state}</Badge>
                  <Badge tone="info">{item.target_provider_label ?? item.target_provider}</Badge>
                </div>
                <p className="mt-1 text-base text-fg-muted">{target?.label ?? item.target_system_id} Â· generation {item.generation} Â· updated {new Date(item.updated_at).toLocaleString()}</p>
                <p className="mt-1 truncate font-mono text-xs text-fg-subtle" title={serviceSource(item.source)}>{serviceSource(item.source)}</p>
              </div>
              <div className="flex items-center gap-1">
                <Button variant="ghost" size="sm" tooltip="Show immutable generation and event history" onPress={() => setHistoryId(historyId === item.id ? "" : item.id)}><History className="size-4" /> History</Button>
                {canManage && item.desired_state === "present" && <IconButton icon={Pencil} label={`Edit ${service?.label ?? item.service_id}`} onPress={() => setEditing(item)} />}
                {canManage && item.desired_state === "present" && item.target_provider === "incus" && <IconButton icon={Trash2} label={`Request removal of ${service?.label ?? item.service_id}`}
                  onPress={() => {
                    if (window.confirm("Request a reviewed stop and removal of this Incus workload? Its deployment data volume is retained. Provider changes occur only after AgentHub approval and provider read-back.")) remove.mutate(item);
                  }} />}
              </div>
            </div>
            <DeploymentExecutionStatus tenantId={tenantId} deployment={item} />
            <DeploymentReviewPanel tenantId={tenantId} deployment={item} canManage={canManage} />
            <details className="mt-3">
              <summary className="cursor-pointer text-xs text-fg-muted">Configuration</summary>
              <div className="mt-2 text-xs text-fg-subtle">Target settings</div>
              <pre className="mt-1 max-h-32 overflow-auto rounded-md bg-canvas p-3 text-xs text-fg-muted">{JSON.stringify(item.target_config, null, 2)}</pre>
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
  tenantId: string; services: DeploymentService[]; targets: { id: string; label: string; provider: string; provider_label?: string; site: string; state: string }[];
  deployment?: Deployment; onClose: () => void;
}) {
  const [serviceId, setServiceId] = useState(deployment?.service_id ?? services[0]?.id ?? "");
  const service = services.find((item) => item.id === serviceId);
  const compatibleTargets = targets.filter((target) => supportsRuntimeTarget(service?.runtime_contract, target.provider));
  const schema = deployment?.config_schema ?? service?.config_schema;
  const source = deployment?.source ?? service?.deployment_source;
  const [targetId, setTargetId] = useState(deployment?.target_system_id ?? compatibleTargets[0]?.id ?? "");
  const [storagePool, setStoragePool] = useState(deployment?.target_config.storage?.pool ?? "");
  const [networkName, setNetworkName] = useState(deployment?.target_config.network?.name ?? "");
  const selectedTarget = compatibleTargets.find((target) => target.id === targetId);
  const targetConfigRequired = !deployment && requiresIncusTargetConfiguration(service?.runtime_contract);
  const storageRequired = targetConfigRequired;
  const networkRequired = targetConfigRequired;
  const targetStorage = useDeploymentTargetStorage(tenantId, targetId,
    targetConfigRequired && selectedTarget?.provider === "incus" && selectedTarget.state === "active");
  const targetNetworks = useDeploymentTargetNetworks(tenantId, targetId,
    targetConfigRequired && selectedTarget?.provider === "incus" && selectedTarget.state === "active");
  const [configText, setConfigText] = useState(() => JSON.stringify(deployment?.config ?? service?.default_config ?? {}, null, 2));
  const [schemaValid, setSchemaValid] = useState(true);
  const setSchemaValidity = useCallback((value: boolean) => setSchemaValid(value), []);
  const create = useCreateDeployment(tenantId);
  const update = useUpdateDeployment(tenantId);
  const config = (() => { try { return JSON.parse(configText) as unknown; } catch { return undefined; } })();
  const valid = isObject(config);
  const configObject = isObject(config) ? config : {};
  const busy = create.isPending || update.isPending;
  const storagePoolValid = !storageRequired || !!targetStorage.data?.items.some((item) => item.name === storagePool);
  const networkValid = !networkRequired || !!targetNetworks.data?.items.some((item) => item.name === networkName && (item.type === "bridge" || item.type === "ovn"));
  return <Drawer isOpen onOpenChange={(open) => !open && onClose()} title={deployment ? `Edit ${service?.label ?? deployment.service_id}` : "Request deployment"}
    subtitle="This records desired configuration and a pinned catalog revision. Provider execution is partial; application health and full service readiness are not yet supported."
    footer={<>
      <div className="flex-1" />
      <Button variant="ghost" tooltip="Close without saving" onPress={onClose}>Cancel</Button>
      <Button variant="primary" tooltip={deployment ? "Append a generation using the current ETag" : "Record desired service configuration"} isDisabled={(!deployment && !service) || !valid || !schemaValid || busy || (!deployment && (!targetId || !compatibleTargets.some((target) => target.id === targetId) || !storagePoolValid || !networkValid || targetStorage.isPending || targetNetworks.isPending))}
        onPress={() => {
          if (!isObject(config) || !schemaValid) return;
          if (deployment) update.mutate({ deployment, config }, { onSuccess: onClose });
          else if (service) {
            const target_config: DeploymentTargetConfig = {
              ...(storageRequired ? { storage: { pool: storagePool } } : {}),
              ...(networkRequired ? { network: { name: networkName } } : {}),
            };
            create.mutate({ service_id: service.id, target_system_id: targetId, desired_state: "present", config, target_config }, { onSuccess: onClose });
          }
        }}>{deployment ? "Save generation" : "Record request"}</Button>
    </>}>
    <div className="grid gap-4 px-6 py-5">
      {!deployment && <>
        <Select label="Service" value={serviceId} options={services.map((item) => ({ id: item.id, label: `${item.label} Â· ${item.kind}` }))}
          onChange={(id) => {
            setServiceId(id);
            const selected = services.find((item) => item.id === id);
            setConfigText(JSON.stringify(selected?.default_config ?? {}, null, 2));
            const nextTarget = targets.find((target) => supportsRuntimeTarget(selected?.runtime_contract, target.provider));
            setTargetId(nextTarget?.id ?? "");
            setStoragePool("");
            setNetworkName("");
          }} />
        <Select label="Target system" value={targetId} options={compatibleTargets.map((target) => ({ id: target.id,
          label: `${target.provider_label ?? target.provider} Â· ${target.label} Â· ${target.site}${target.state === "planned" ? " Â· connection unverified" : ""}` }))} onChange={(id) => { setTargetId(id); setStoragePool(""); setNetworkName(""); }} />
        {compatibleTargets.length === 0 && <p className="text-base text-fg-muted">No assigned target supports this service runtime declaration.</p>}
        {storageRequired && <>
          {selectedTarget?.state !== "active" ? <p className="text-base text-warning">Connect and verify this target to discover its live storage pools.</p>
            : targetStorage.isPending ? <p className="text-base text-fg-muted">Loading storage pools from the selected target…</p>
              : targetStorage.isError ? <p role="alert" className="text-base text-danger">Storage pools could not be loaded: {targetStorage.error.message}</p>
                : (targetStorage.data?.items.length ?? 0) === 0 ? <p className="text-base text-warning">The target reports no storage pools.</p>
                  : <Select label="Incus storage pool" value={storagePool}
                    options={(targetStorage.data?.items ?? []).map((pool) => ({ id: pool.name, label: `${pool.name} · ${pool.type}` }))}
                    onChange={setStoragePool} />}
          <p className="text-xs text-fg-subtle">Pool choice is read from the selected target and saved with this deployment generation. It cannot be changed after creation until data migration is supported.</p>
        </>}
        {networkRequired && <>
          {selectedTarget?.state !== "active" ? <p className="text-base text-warning">Connect and verify this target to discover its managed networks.</p>
            : targetNetworks.isPending ? <p className="text-base text-fg-muted">Loading managed networks from the selected target…</p>
              : targetNetworks.isError ? <p role="alert" className="text-base text-danger">Managed networks could not be loaded: {targetNetworks.error.message}</p>
                : (targetNetworks.data?.items.length ?? 0) === 0 ? <p className="text-base text-warning">The target reports no selectable bridge or OVN networks.</p>
                  : <Select label="Incus managed network" value={networkName}
                    options={(targetNetworks.data?.items ?? []).map((network) => ({ id: network.name, label: `${network.name} · ${network.type}` }))}
                    onChange={setNetworkName} />}
          <p className="text-xs text-fg-subtle">The selected network is pinned into this generation and revalidated on save. This configures private NIC attachment; edge ingress and provider execution remain unavailable until their runtime contracts are implemented.</p>
        </>}
      </>}
      {source && <div className="rounded-lg border border-line p-3">
        <div className="text-base font-medium text-fg">Pinned source</div>
        <div className="mt-1 break-all font-mono text-xs text-fg-muted">{serviceSource(source)}</div>
        {service?.description && <div className="mt-2 text-xs text-fg-subtle">{service.description}</div>}
      </div>}
      {schema && valid
        ? <ServiceConfigForm key={deployment?.id ?? serviceId} schema={schema} value={configObject} onChange={(next) => setConfigText(JSON.stringify(next, null, 2))} onValidityChange={setSchemaValidity} />
        : <p className="text-base text-fg-muted">{valid ? "This service has no editable schema. Use the advanced JSON editor." : "Enter a valid JSON object in the advanced editor."}</p>}
      <details className="rounded-lg border border-line p-3">
        <summary className="cursor-pointer text-base font-medium text-fg">Advanced JSON editor</summary>
        <TextField label="Configuration (JSON)" multiline mono value={configText} onChange={setConfigText}
          description={valid ? "The server validates these values against the pinned service schema when saved." : "Enter a valid JSON object."} />
      </details>
      {schema && <details className="rounded-lg border border-line p-3">
        <summary className="cursor-pointer text-base font-medium text-fg">Configuration schema and required fields</summary>
        <p className="mt-2 text-xs text-fg-muted">Secret defaults are hidden. Vaultix references use vaultix://&lt;environment&gt;/&lt;folder&gt;/&lt;key&gt; and resolve inside this tenant's Vaultix project. SOPS references are stored but are not resolved by the deployment worker yet. Never enter secret values.</p>
        <pre className="mt-2 max-h-64 overflow-auto rounded-md bg-canvas p-3 text-xs text-fg-muted">{JSON.stringify(schema, null, 2)}</pre>
      </details>}
      {deployment && <div className="flex items-center gap-2 text-xs text-fg-subtle"><Clock3 className="size-3.5" /> Saving appends generation {deployment.generation + 1}; previous generations remain in history.</div>}
    </div>
  </Drawer>;
}

function DeploymentReviewPanel({ tenantId, deployment, canManage }: { tenantId: string; deployment: Deployment; canManage: boolean }) {
  const review = useDeploymentReview(tenantId, deployment.id);
  const events = useDeploymentEvents(tenantId, deployment.id);
  const prepare = usePrepareDeploymentReview(tenantId, deployment.id);
  const authorize = useAuthorizeDeploymentReview(tenantId, deployment.id);
  const state = review.data?.review?.state ?? review.data?.state ?? "not_requested";
  const canRequest = canManage && !prepare.isPending && (state === "not_requested" || state === "failed");
  const canAuthorize = canManage && !authorize.isPending && state === "pending_review" && review.data?.agenthub_status === "passed";
  const lastReviewDecision = [...(events.data?.items ?? [])].filter((event) => event.generation === deployment.generation)
    .reverse().find((event) => event.event_type === "review_allowed" || event.event_type === "review_denied");
  return <section className="mt-3 flex flex-wrap items-center gap-2 rounded-md border border-line p-3" aria-label="AgentHub review">
    {state === "pending_review" ? <>
      <CheckCircle2 className="size-4 text-info" />
      <span className="text-base text-fg">AgentHub ticket submitted</span>
      <Badge tone="info">{review.data?.agenthub_status ?? "unavailable"}</Badge>
    </> : state === "preparing" ? <span className="text-base text-fg-muted">Preparing the AgentHub ticket…</span>
      : state === "failed" ? <span role="alert" className="text-base text-danger">AgentHub review request failed ({review.data?.review?.failure_code ?? "unknown"}).</span>
        : <span className="text-base text-fg-muted">No AgentHub review ticket for this generation.</span>}
    {canRequest && <Button variant="secondary" size="sm" tooltip="Create an AgentHub ticket for this exact generation. This does not execute it."
      onPress={() => prepare.mutate(deployment.generation)}>{state === "failed" ? "Retry review request" : "Request AgentHub review"}</Button>}
    {canAuthorize && <Button variant="secondary" size="sm" tooltip="Recheck the current ticket, policy, mapped system and quorum with AgentHub. This does not execute the deployment."
      onPress={() => authorize.mutate(deployment.generation)}>Verify authorization</Button>}
    {review.isError && <span role="alert" className="text-xs text-danger">Review status could not be loaded: {review.error.message}</span>}
    {review.data?.review && <span className="font-mono text-xs text-fg-subtle">ticket {review.data.review.agenthub_ticket_id} · action {review.data.review.action_sha256.slice(0, 12)}</span>}
    {lastReviewDecision?.event_type === "review_allowed" && lastReviewDecision.review_provenance && <span className="basis-full text-xs text-fg-subtle">
      Authorization recorded {new Date(lastReviewDecision.recorded_at).toLocaleString()} · policy v{lastReviewDecision.review_provenance.policy_version} · {lastReviewDecision.review_provenance.matched_rules.join(", ")}.
    </span>}
    {lastReviewDecision?.event_type === "review_denied" && <span className="basis-full text-xs text-danger">
      Latest authorization check did not pass ({lastReviewDecision.failure_code ?? "unknown"}).
    </span>}
    <p className="basis-full text-xs text-fg-subtle">A review ticket or recorded authorization does not execute the deployment. The reconciler must reauthorize immediately before provider changes.</p>
  </section>;
}

function DeploymentExecutionStatus({ tenantId, deployment }: { tenantId: string; deployment: Deployment }) {
  const events = useDeploymentEvents(tenantId, deployment.id);
  const failure = currentGenerationFailure(events.data?.items ?? [], deployment.generation);
  if (!failure) return null;
  return <div role="status" className="mt-3 rounded-md border border-warning/40 bg-warning/5 p-3 text-base">
    <p className="font-medium text-fg">Deployment generation {deployment.generation} is incomplete</p>
    <p className="mt-1 text-fg-muted">{deploymentFailureMessage(failure.failure_code)}</p>
    <p className="mt-1 text-xs text-fg-subtle">Recorded {new Date(failure.recorded_at).toLocaleString()}.</p>
  </div>;
}

function DeploymentHistory({ tenantId, deployment }: { tenantId: string; deployment: Deployment }) {
  const query = useDeploymentEvents(tenantId, deployment.id);
  if (query.isPending) return <p className="mt-3 text-xs text-fg-muted">Loading event history…</p>;
  if (query.error) return <p className="mt-3 text-xs text-danger">{query.error.message}</p>;
  return <ol className="mt-3 grid gap-2 border-l border-line pl-4">
    {(query.data?.items ?? []).map((event) => {
      const summary = deploymentEventSummary(event);
      return <li key={event.id} className="text-xs text-fg-muted">
        <span className="font-medium text-fg">{deploymentEventLabel(event.event_type)}</span> Â· generation {event.generation} Â· {event.actor} Â· {new Date(event.recorded_at).toLocaleString()}
        {summary && <p className="mt-1">{summary}</p>}
      </li>;
    })}
  </ol>;
}
