import { useState } from "react";
import { Badge } from "../../../../lib/design/components/badge/badge";
import { Button } from "../../../../lib/design/components/button/button";
import { useInfraGuests, useInfraOverview } from "../../../../lib/hooks/use-infra";
import {
  usePlatformServiceMutation,
  usePlatformServiceObservations,
  usePlatformServices,
  type PlatformService,
  type PlatformServiceInput,
} from "../../../../lib/hooks/use-platform-health";

const blank: PlatformServiceInput = {
  name: "",
  kind: "",
  alias: "",
  url: "",
  host: "",
  tcp: "",
  instance: "",
  server_id: "",
  guest_id: "",
  timeout_seconds: 3,
  interval_seconds: 60,
  reason: "",
};

const stateTone = {
  operational: "success",
  down: "danger",
  stale: "warning",
  unknown: "neutral",
  disabled: "neutral",
} as const;

const systemID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** Operator-owned health registry editor; definitions are persisted individually, with audit. */
export function PlatformServiceRegistry({ canWrite }: { canWrite: boolean }) {
  const query = usePlatformServices();
  const actions = usePlatformServiceMutation();
  const systems = useInfraOverview(canWrite);
  const [draft, setDraft] = useState<PlatformServiceInput | null>(null);
  const [editing, setEditing] = useState<PlatformService | null>(null);
  const [disabling, setDisabling] = useState<PlatformService | null>(null);
  const [disableReason, setDisableReason] = useState("");
  const [target, setTarget] = useState<"http" | "tcp">("http");

  const guests = useInfraGuests(
    draft && systemID.test(draft.server_id ?? "") ? draft.server_id! : "",
  );
  const providerSystems =
    systems.data?.providers.flatMap((provider) =>
      provider.nodes
        .filter((node) => node.source === "api")
        .map((node) => ({ id: node.id, label: `${node.label} (${provider.label})` })),
    ) ?? [];

  const update = <K extends keyof PlatformServiceInput>(
    key: K,
    value: PlatformServiceInput[K],
  ) => setDraft((current) => (current ? { ...current, [key]: value } : current));

  const beginEdit = (service: PlatformService) => {
    setEditing(service);
    setTarget(service.tcp ? "tcp" : "http");
    setDraft({
      name: service.name,
      kind: service.kind,
      alias: service.alias ?? "",
      url: service.url ?? "",
      host: service.host ?? "",
      tcp: service.tcp ?? "",
      instance: service.instance ?? "",
      server_id: service.server_id ?? "",
      guest_id: service.guest_id ?? "",
      timeout_seconds: service.timeout_seconds,
      interval_seconds: service.interval_seconds,
      reason: "",
    });
  };

  const canSubmit = Boolean(
    draft &&
      draft.name.trim() &&
      draft.kind.trim() &&
      (target === "http" ? draft.url?.trim() : draft.tcp?.trim()) &&
      Boolean(draft.server_id) === Boolean(draft.guest_id) &&
      Number.isInteger(draft.timeout_seconds) &&
      draft.timeout_seconds >= 1 &&
      draft.timeout_seconds <= 10 &&
      Number.isInteger(draft.interval_seconds) &&
      draft.interval_seconds >= 30 &&
      draft.interval_seconds <= 86400 &&
      draft.reason.trim().length >= 5,
  );

  const submit = () => {
    if (!draft || !canSubmit) return;
    const payload = {
      ...draft,
      url: target === "http" ? draft.url : "",
      tcp: target === "tcp" ? draft.tcp : "",
      ...(editing ? { id: editing.id, revision: editing.revision } : {}),
    };
    actions.save.mutate(payload, {
      onSuccess: () => {
        setDraft(null);
        setEditing(null);
      },
    });
  };

  if (query.isError) {
    return <p role="alert" className="p-4 text-sm text-danger">{query.error.message}</p>;
  }

  return (
    <section className="space-y-4 border-t border-line p-4" aria-label="Platform health services">
      <header>
        <h2 className="font-semibold">Platform health services</h2>
        <p className="text-sm text-fg-muted">
          Configure service targets, check cadence, latest observations, and optional provider resources.
        </p>
      </header>

      {query.isLoading && <p className="text-sm text-fg-muted">Loading service registry…</p>}

      <ul className="space-y-2">
        {query.data?.items.map((service) => (
          <li
            key={service.id}
            className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-line p-3"
          >
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-medium">{service.alias || service.name}</span>
                <Badge tone={stateTone[service.state]}>{service.state}</Badge>
              </div>
              <p className="truncate font-mono text-xs text-fg-subtle">
                {service.url || service.tcp || "No target"}
              </p>
              <p className="text-xs text-fg-subtle">
                {service.kind} · every {service.interval_seconds}s · {observationSummary(service)}
              </p>
              <ObservationHistory serviceId={service.id} />
            </div>
            {canWrite && (
              <div className="flex gap-2">
                {service.enabled && (
                  <Button
                    size="sm"
                    variant="ghost"
                    tooltip="Run a bounded health check and record its result"
                    isDisabled={actions.check.isPending}
                    onPress={() => actions.check.mutate(service.id)}
                  >
                    Check now
                  </Button>
                )}
                <Button
                  size="sm"
                  variant="ghost"
                  tooltip={service.enabled ? "Edit this platform service" : "Edit and re-enable this service"}
                  onPress={() => beginEdit(service)}
                >
                  {service.enabled ? "Edit" : "Re-enable"}
                </Button>
                {service.enabled && (
                  <Button
                    size="sm"
                    variant="ghost"
                    tooltip="Soft-disable this service while preserving its observations"
                    onPress={() => {
                      setDisabling(service);
                      setDisableReason("");
                    }}
                  >
                    Disable
                  </Button>
                )}
              </div>
            )}
          </li>
        ))}
      </ul>

      {disabling && (
        <form
          className="flex flex-wrap items-end gap-2 rounded-lg border border-line p-3"
          onSubmit={(event) => {
            event.preventDefault();
            actions.disable.mutate(
              { ...disabling, reason: disableReason },
              { onSuccess: () => setDisabling(null) },
            );
          }}
        >
          <label className="min-w-64 flex-1 text-sm">
            Reason for disabling {disabling.name}
            <input
              className="mt-1 w-full rounded-md border border-line bg-surface px-3 py-2"
              minLength={5}
              required
              value={disableReason}
              onChange={(event) => setDisableReason(event.target.value)}
            />
          </label>
          <Button
            size="sm"
            variant="primary"
            tooltip="Disable this service and preserve its observations"
            isDisabled={actions.disable.isPending || disableReason.trim().length < 5}
            type="submit"
          >
            Confirm disable
          </Button>
          <Button
            size="sm"
            variant="ghost"
            tooltip="Cancel disabling this service"
            onPress={() => setDisabling(null)}
          >
            Cancel
          </Button>
        </form>
      )}

      {canWrite &&
        (draft ? (
          <div className="grid gap-3 rounded-lg border border-line p-3 sm:grid-cols-2">
            <TextField label="Display name" value={draft.name} onChange={(value) => update("name", value)} />
            <TextField
              label="Service kind"
              placeholder="identity, secrets, edge, database, operator"
              value={draft.kind}
              onChange={(value) => update("kind", value)}
            />
            <TextField label="Alias (optional)" value={draft.alias ?? ""} onChange={(value) => update("alias", value)} />
            <label className="text-sm">
              Target protocol
              <select
                className="mt-1 w-full rounded-md border border-line bg-surface px-3 py-2"
                value={target}
                onChange={(event) => {
                  const value = event.target.value as "http" | "tcp";
                  setTarget(value);
                  update(value === "http" ? "tcp" : "url", "");
                }}
              >
                <option value="http">HTTP / HTTPS</option>
                <option value="tcp">TCP</option>
              </select>
            </label>
            {target === "http" ? (
              <TextField
                label="Health URL"
                placeholder="https://service.example/health"
                value={draft.url ?? ""}
                onChange={(value) => update("url", value)}
              />
            ) : (
              <TextField
                label="TCP host:port"
                placeholder="service.internal:5432"
                value={draft.tcp ?? ""}
                onChange={(value) => update("tcp", value)}
              />
            )}
            <TextField label="HTTP Host header" value={draft.host ?? ""} onChange={(value) => update("host", value)} />
            <label className="text-sm">
              System UUID (optional)
              <input
                className="mt-1 w-full rounded-md border border-line bg-surface px-3 py-2 font-mono"
                list="platform-health-systems"
                value={draft.server_id ?? ""}
                onChange={(event) => {
                  update("server_id", event.target.value);
                  update("guest_id", "");
                }}
              />
              <datalist id="platform-health-systems">
                {providerSystems.map((system) => (
                  <option key={system.id} value={system.id}>{system.label}</option>
                ))}
              </datalist>
              <span className="mt-1 block text-xs text-fg-subtle">
                Suggestions come from configured Incus and Proxmox connections.
              </span>
            </label>
            {draft.server_id && (
              <label className="text-sm">
                Provider guest ID
                <input
                  className="mt-1 w-full rounded-md border border-line bg-surface px-3 py-2 font-mono"
                  list="platform-health-guests"
                  value={draft.guest_id ?? ""}
                  onChange={(event) => update("guest_id", event.target.value)}
                />
                <datalist id="platform-health-guests">
                  {guests.data?.items.map((guest) => (
                    <option key={guest.id} value={guest.id}>
                      {guest.name} · {guest.kind} · {guest.status}
                    </option>
                  ))}
                </datalist>
                {guests.isError && (
                  <span className="mt-1 block text-xs text-fg-subtle">
                    Guest suggestions are unavailable; enter the provider guest ID directly.
                  </span>
                )}
              </label>
            )}
            <NumberField
              label="Probe timeout (seconds)"
              min={1}
              max={10}
              value={draft.timeout_seconds}
              onChange={(value) => update("timeout_seconds", value)}
            />
            <NumberField
              label="Probe interval (seconds)"
              min={30}
              max={86400}
              value={draft.interval_seconds}
              onChange={(value) => update("interval_seconds", value)}
            />
            <TextField
              label="Audit reason"
              value={draft.reason}
              onChange={(value) => update("reason", value)}
              className="sm:col-span-2"
            />
            <div className="flex gap-2 sm:col-span-2">
              <Button
                size="sm"
                variant="primary"
                tooltip="Save this service definition"
                isDisabled={actions.save.isPending || !canSubmit}
                onPress={submit}
              >
                Save service
              </Button>
              <Button
                size="sm"
                variant="ghost"
                tooltip="Discard this edit"
                onPress={() => {
                  setDraft(null);
                  setEditing(null);
                }}
              >
                Cancel
              </Button>
            </div>
          </div>
        ) : (
          <Button
            size="sm"
            variant="ghost"
            tooltip="Add a configurable platform health target"
            onPress={() => {
              setEditing(null);
              setTarget("http");
              setDraft({ ...blank });
            }}
          >
            Add service
          </Button>
        ))}
    </section>
  );
}

function observationSummary(service: PlatformService): string {
  const observation = service.observation;
  if (!observation) return "No observation yet";
  const latency = observation.latency_ms === undefined ? "" : ` · ${observation.latency_ms} ms`;
  return `${observation.detail}${latency}`;
}

function ObservationHistory({ serviceId }: { serviceId: string }) {
  const [open, setOpen] = useState(false);
  const history = usePlatformServiceObservations(serviceId, open);
  return (
    <details className="mt-2 text-xs" onToggle={(event) => setOpen(event.currentTarget.open)}>
      <summary className="cursor-pointer text-fg-muted">Recent observations</summary>
      {history.isError && <p role="alert" className="mt-2 text-danger">{history.error.message}</p>}
      {history.isLoading && <p className="mt-2 text-fg-subtle">Loading observation history…</p>}
      {history.data && history.data.items.length === 0 && <p className="mt-2 text-fg-subtle">No observations recorded.</p>}
      <ol className="mt-2 space-y-1">
        {history.data?.items.map((item) => (
          <li key={item.id} className="flex flex-wrap gap-x-3 gap-y-1 text-fg-subtle">
            <time dateTime={item.observed_at}>{new Date(item.observed_at).toLocaleString()}</time>
            <span>{item.healthy ? "Reachable" : item.configured ? "Unreachable" : "Not configured"}</span>
            {item.latency_ms !== undefined && <span>{item.latency_ms} ms</span>}
            <span>{item.detail}</span>
            {item.resource && <span>Provider: {item.resource.observed ? item.resource.status ?? "observed" : item.resource.detail}</span>}
          </li>
        ))}
      </ol>
    </details>
  );
}

function TextField({
  label,
  value,
  onChange,
  placeholder,
  className,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  className?: string;
}) {
  return (
    <label className={`text-sm ${className ?? ""}`}>
      {label}
      <input
        className="mt-1 w-full rounded-md border border-line bg-surface px-3 py-2"
        value={value}
        placeholder={placeholder}
        onChange={(event) => onChange(event.target.value)}
      />
    </label>
  );
}

function NumberField({
  label,
  min,
  max,
  value,
  onChange,
}: {
  label: string;
  min: number;
  max: number;
  value: number;
  onChange: (value: number) => void;
}) {
  return (
    <label className="text-sm">
      {label}
      <input
        className="mt-1 w-full rounded-md border border-line bg-surface px-3 py-2"
        type="number"
        min={min}
        max={max}
        value={value}
        onChange={(event) => onChange(Number(event.target.value))}
      />
    </label>
  );
}
