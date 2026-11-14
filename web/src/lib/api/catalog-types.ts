export type Lock = "locked" | "overridable" | "mergeable" | "bounded";
export type Layer = "system" | "plan" | "tenant" | "workspace";
export type Json = string | number | boolean | null | Json[] | { [k: string]: Json };

export type Plan = { id: string; label: string; description: string; rank: number; /** omitted by the API without catalog.prices.read */ price_cents?: number; public: boolean; active: boolean;
  /** The coverage tier this plan includes. */
  coverage: string };

/** Support and operation sold beside a plan, generated from the coverage_tier action types. */
export type { CoverageTier } from "./actions.gen";

export type CatalogDeploymentSource = {
  kind: "github" | "git" | "oci";
  reference: string;
  commit?: string;
  context_path?: string;
  credential_ref?: string;
};

export type CatalogRuntimePort = { name: string; protocol: "tcp" | "udp"; container_port: number };
export type CatalogRuntimeContractV1 = {
  version: 1; kind: "incus_oci_container"; providers: ["incus"];
};
export type CatalogRuntimeContractV2 = {
  version: 2; kind: "incus_oci_container"; providers: ["incus"];
  resources: { cpu_cores: number; memory_mib: number };
  storage: { size_gib: number; mount_path: string };
  network: { exposure: "private" | "edge"; ports: CatalogRuntimePort[] };
  health: { kind: "tcp" | "http"; port: number; path?: string; interval_seconds: number; timeout_seconds: number; failure_threshold: number };
};
export type CatalogRuntimeEnvironmentBinding = { name: string; config_path: string };
export type CatalogRuntimeContractV3 = Omit<CatalogRuntimeContractV2, "version"> & {
  version: 3; environment: CatalogRuntimeEnvironmentBinding[];
};
export type CatalogRuntimeContract = CatalogRuntimeContractV1 | CatalogRuntimeContractV2 | CatalogRuntimeContractV3;

export type CatalogService = {
  id: string; label: string; description: string; kind: string; docs_url: string;
  config_schema: { properties?: Record<string, { type?: string; title?: string }> };
  default_config: Record<string, Json>; deployment_source: CatalogDeploymentSource | null;
  runtime_contract?: CatalogRuntimeContract | null; required: boolean; active: boolean;
};

export type PlanService = { plan_id: string; service_id: string; enabled: boolean; lock: Lock; limits: Record<string, Json> };

/** A customer, its contacts and endpoints, its provisioning requests and its merged configuration are
 * generated from the tenancy.customer action types (contacts, endpoints and notes are left out by the API
 * without customers.contacts.read and customers.notes.read). */
export type { CatalogEffective, CatalogEffectiveConfig, Customer, CustomerContact, CustomerEndpoint, ProvisionIntent } from "./actions.gen";

export type Items<T> = { items: T[] };
