import type { CatalogDeploymentSource, CatalogRuntimeContract, CatalogRuntimeContractV2 } from "../../../../lib/api/catalog-types";

export type RuntimeKindDraft = "none" | "incus_oci_container";

export function runtimeKindDraft(runtime?: CatalogRuntimeContract | null): RuntimeKindDraft {
  return runtime?.kind === "incus_oci_container" && runtime.providers.length === 1 && runtime.providers[0] === "incus" ? runtime.kind : "none";
}

export function runtimeContractFromDraft(kind: RuntimeKindDraft, source: CatalogDeploymentSource | null, raw: string): CatalogRuntimeContract | null {
  if (kind === "none" || source?.kind !== "oci") return null;
  try {
    const value = JSON.parse(raw) as CatalogRuntimeContract;
    return validRuntimeContract(value) ? value : null;
  } catch {
    return null;
  }
}

export function validRuntimeDraft(kind: RuntimeKindDraft, source: CatalogDeploymentSource | null, raw: string): boolean {
  return kind === "none" || (kind === "incus_oci_container" && source?.kind === "oci" && runtimeContractFromDraft(kind, source, raw) !== null);
}

export function exampleRuntimeContract(): CatalogRuntimeContractV2 {
  return {
    version: 2,
    kind: "incus_oci_container",
    providers: ["incus"],
    resources: { cpu_cores: 2, memory_mib: 2048 },
    storage: { size_gib: 10, mount_path: "/var/lib/shippin/data" },
    network: { exposure: "private", ports: [{ name: "service", protocol: "tcp", container_port: 8080 }] },
    health: { kind: "tcp", port: 8080, interval_seconds: 30, timeout_seconds: 5, failure_threshold: 3 },
  };
}

/** Starts a user-authored recipe without implying app-specific defaults. */
export function emptyRuntimeContractDraft(): CatalogRuntimeContractV2 {
  return {
    version: 2,
    kind: "incus_oci_container",
    providers: ["incus"],
    resources: { cpu_cores: 0, memory_mib: 0 },
    storage: { size_gib: 0, mount_path: "" },
    network: { exposure: "private", ports: [{ name: "", protocol: "tcp", container_port: 0 }] },
    health: { kind: "tcp", port: 0, interval_seconds: 0, timeout_seconds: 0, failure_threshold: 0 },
  };
}

export function validRuntimeContract(value: unknown): value is CatalogRuntimeContract {
  if (typeof value !== "object" || value === null || Array.isArray(value)) return false;
  const contract = value as CatalogRuntimeContract;
  if (contract.kind !== "incus_oci_container" || !Array.isArray(contract.providers) || contract.providers.length !== 1 || contract.providers[0] !== "incus") return false;
  if (contract.version === 1) return hasOnlyKeys(contract, ["version", "kind", "providers"]);
  if (contract.version !== 2 || !hasOnlyKeys(contract, ["version", "kind", "providers", "resources", "storage", "network", "health"])) return false;
  const { resources, storage, network, health } = contract;
  if (!isRecord(resources) || !hasOnlyKeys(resources, ["cpu_cores", "memory_mib"]) ||
      !Number.isInteger(resources.cpu_cores) || resources.cpu_cores < 1 || resources.cpu_cores > 64 ||
      !Number.isInteger(resources.memory_mib) || resources.memory_mib < 128 || resources.memory_mib > 1_048_576) return false;
  if (!isRecord(storage) || !hasOnlyKeys(storage, ["size_gib", "mount_path"]) || !Number.isInteger(storage.size_gib) || storage.size_gib < 1 || storage.size_gib > 65_536 ||
      typeof storage.mount_path !== "string" || !validAbsolutePath(storage.mount_path)) return false;
  if (!isRecord(network) || !hasOnlyKeys(network, ["exposure", "ports"]) || (network.exposure !== "private" && network.exposure !== "edge") || !Array.isArray(network.ports) || network.ports.length < 1 || network.ports.length > 32) return false;
  const names = new Set<string>();
  const ports = new Set<string>();
  for (const port of network.ports) {
    if (!isRecord(port) || !hasOnlyKeys(port, ["name", "protocol", "container_port"]) || typeof port.name !== "string" ||
        !/^[a-z][a-z0-9-]{0,31}$/.test(port.name) || !["tcp", "udp"].includes(port.protocol) ||
        !Number.isInteger(port.container_port) || port.container_port < 1 || port.container_port > 65535 ||
        names.has(port.name) || ports.has(`${port.protocol}:${port.container_port}`)) return false;
    names.add(port.name);
    ports.add(`${port.protocol}:${port.container_port}`);
  }
  if (!isRecord(health) || !hasOnlyKeys(health, health.kind === "http" ? ["kind", "port", "path", "interval_seconds", "timeout_seconds", "failure_threshold"] : ["kind", "port", "interval_seconds", "timeout_seconds", "failure_threshold"]) ||
      !["tcp", "http"].includes(health.kind) || !Number.isInteger(health.port) || health.port < 1 || health.port > 65535 ||
      !Number.isInteger(health.interval_seconds) || health.interval_seconds < 5 || health.interval_seconds > 300 ||
      !Number.isInteger(health.timeout_seconds) || health.timeout_seconds < 1 || health.timeout_seconds > 60 || health.timeout_seconds >= health.interval_seconds ||
      !Number.isInteger(health.failure_threshold) || health.failure_threshold < 1 || health.failure_threshold > 10 ||
      !network.ports.some((port) => port.protocol === "tcp" && port.container_port === health.port)) return false;
  if (health.kind === "http") return typeof health.path === "string" && validAbsolutePath(health.path);
  return health.path === undefined || health.path === "";
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function hasOnlyKeys(value: Record<string, unknown>, keys: string[]): boolean {
  return Object.keys(value).every((key) => keys.includes(key));
}

function validAbsolutePath(value: string): boolean {
  return value.startsWith("/") && value !== "/" && !/[\\?#\r\n\0]/.test(value) &&
    value.split("/").slice(1).every((segment) => segment !== "" && segment !== "." && segment !== "..");
}
