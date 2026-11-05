import type { Dispatch, SetStateAction } from "react";
import type { CatalogRuntimeContractV2, CatalogRuntimePort } from "../../../../lib/api/catalog-types";
import { Button } from "../../../../lib/design/components/button/button";
import { Select } from "../../../../lib/design/components/select/select";
import { TextField } from "../../../../lib/design/components/field/text-field";

type Props = {
  value: CatalogRuntimeContractV2;
  onChange: Dispatch<SetStateAction<CatalogRuntimeContractV2>>;
};

/** Structured editor for the versioned Incus OCI runtime declaration. */
export function RuntimeRecipeForm({ value, onChange }: Props) {
  const updatePort = (index: number, change: (port: CatalogRuntimePort) => CatalogRuntimePort) => {
    onChange((current) => ({
      ...current,
      network: { ...current.network, ports: current.network.ports.map((port, i) => i === index ? change(port) : port) },
    }));
  };

  const addPort = () => onChange((current) => {
    let suffix = 1;
    while (current.network.ports.some((port) => port.name === `port-${suffix}`)) suffix += 1;
    return {
      ...current,
      network: {
        ...current.network,
        ports: [...current.network.ports, { name: `port-${suffix}`, protocol: "tcp", container_port: 0 }],
      },
    };
  });

  return (
    <div className="grid gap-4">
      <p className="text-xs text-fg-muted">Set the resource, storage, network, and health contract. Saving records desired configuration; it does not enable deployment.</p>
      <div className="grid grid-cols-2 gap-3">
        <TextField label="CPU cores" type="number" min={1} max={64} step={1} value={String(value.resources.cpu_cores)} onChange={(v) => onChange((r) => ({ ...r, resources: { ...r.resources, cpu_cores: Number(v) } }))} />
        <TextField label="Memory (MiB)" type="number" min={128} max={1_048_576} step={128} value={String(value.resources.memory_mib)} onChange={(v) => onChange((r) => ({ ...r, resources: { ...r.resources, memory_mib: Number(v) } }))} />
        <TextField label="Persistent storage (GiB)" type="number" min={1} max={65_536} step={1} value={String(value.storage.size_gib)} onChange={(v) => onChange((r) => ({ ...r, storage: { ...r.storage, size_gib: Number(v) } }))} />
        <TextField label="Container mount path" mono value={value.storage.mount_path} onChange={(v) => onChange((r) => ({ ...r, storage: { ...r.storage, mount_path: v } }))} />
      </div>

      <section className="grid gap-3 rounded-md bg-canvas-subtle p-3" aria-label="Network configuration">
        <div className="flex items-center justify-between">
          <h4 className="text-sm font-medium text-fg">Network ports</h4>
          <Button tooltip="Add port" variant="secondary" isDisabled={value.network.ports.length >= 32} onPress={addPort}>Add port</Button>
        </div>
        <Select label="Exposure" options={[{ id: "private", label: "Private" }, { id: "edge", label: "Edge" }]} value={value.network.exposure} onChange={(v) => onChange((r) => ({ ...r, network: { ...r.network, exposure: v as CatalogRuntimeContractV2["network"]["exposure"] } }))} />
        {value.network.ports.map((port, index) => <div key={`${index}-${port.name}`} className="grid grid-cols-[1fr_110px_120px_auto] items-end gap-2">
          <TextField label="Port name" value={port.name} onChange={(v) => updatePort(index, (current) => ({ ...current, name: v }))} />
          <Select label="Protocol" options={[{ id: "tcp", label: "TCP" }, { id: "udp", label: "UDP" }]} value={port.protocol} onChange={(v) => updatePort(index, (current) => ({ ...current, protocol: v as CatalogRuntimePort["protocol"] }))} />
          <TextField label="Container port" type="number" min={1} max={65535} step={1} value={String(port.container_port)} onChange={(v) => updatePort(index, (current) => ({ ...current, container_port: Number(v) }))} />
          <Button tooltip="Remove port" variant="danger" isDisabled={value.network.ports.length <= 1} onPress={() => onChange((r) => ({ ...r, network: { ...r.network, ports: r.network.ports.filter((_, i) => i !== index) } }))}>Remove</Button>
        </div>)}
      </section>

      <section className="grid gap-3 rounded-md bg-canvas-subtle p-3" aria-label="Health check configuration">
        <h4 className="text-sm font-medium text-fg">Health check</h4>
        <div className="grid grid-cols-2 gap-3">
          <Select label="Check type" options={[{ id: "tcp", label: "TCP" }, { id: "http", label: "HTTP" }]} value={value.health.kind} onChange={(v) => onChange((r) => ({ ...r, health: { ...r.health, kind: v as "tcp" | "http", ...(v === "http" ? { path: r.health.path ?? "" } : { path: undefined }) } }))} />
          <TextField label="Health port" type="number" min={1} max={65535} step={1} value={String(value.health.port)} onChange={(v) => onChange((r) => ({ ...r, health: { ...r.health, port: Number(v) } }))} />
          {value.health.kind === "http" && <TextField label="HTTP path" mono value={value.health.path ?? ""} onChange={(v) => onChange((r) => ({ ...r, health: { ...r.health, path: v } }))} />}
          <TextField label="Interval (seconds)" type="number" min={5} max={300} step={1} value={String(value.health.interval_seconds)} onChange={(v) => onChange((r) => ({ ...r, health: { ...r.health, interval_seconds: Number(v) } }))} />
          <TextField label="Timeout (seconds)" type="number" min={1} max={60} step={1} value={String(value.health.timeout_seconds)} onChange={(v) => onChange((r) => ({ ...r, health: { ...r.health, timeout_seconds: Number(v) } }))} />
          <TextField label="Failure threshold" type="number" min={1} max={10} step={1} value={String(value.health.failure_threshold)} onChange={(v) => onChange((r) => ({ ...r, health: { ...r.health, failure_threshold: Number(v) } }))} />
        </div>
      </section>

      <details className="text-xs text-fg-muted">
        <summary className="cursor-pointer">Review saved recipe</summary>
        <pre className="mt-2 max-h-64 overflow-auto rounded bg-canvas-subtle p-3 font-mono text-[11px]">{JSON.stringify(value, null, 2)}</pre>
      </details>
    </div>
  );
}
