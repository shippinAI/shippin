import type { CatalogDeploymentSource as GeneratedDeploymentSource, CatalogLock } from "./actions.gen";

/** A lock mode, generated from catalog.Lock. */
export type Lock = CatalogLock;
export type Layer = "system" | "plan" | "tenant" | "workspace";
export type Json = string | number | boolean | null | Json[] | { [k: string]: Json };

/** A plan with its services (the plan x service matrix), generated from the tenancy/plan action types. The
 * price is left out without catalog.prices.read. */
export type { Plan, PlanService } from "./actions.gen";

/** Support and operation sold beside a plan, generated from the coverage_tier action types. */
export type { CoverageTier } from "./actions.gen";

/** The generated deployment source with its kind narrowed to the ones the server accepts. */
export type CatalogDeploymentSource = Omit<GeneratedDeploymentSource, "kind"> & { kind: "github" | "git" | "oci" };

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

/** A catalog service definition, generated from the catalog_service action types. Its deployment source and
 * runtime contract are the generated shapes; the Catalog* types above narrow them for the service form. */
export type { CatalogService } from "./actions.gen";

/** A customer, its contacts and endpoints, its provisioning requests and its merged configuration are
 * generated from the tenancy.customer action types (contacts, endpoints and notes are left out by the API
 * without customers.contacts.read and customers.notes.read). */
export type { CatalogEffective, CatalogEffectiveConfig, Customer, CustomerContact, CustomerEndpoint, ProvisionIntent } from "./actions.gen";

export type Items<T> = { items: T[] };
