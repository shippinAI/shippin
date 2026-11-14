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

export type Contact = { name: string; email: string; role: string };
export type Endpoint = { kind: string; host: string; note: string };

export type Customer = {
  id: string; name: string; plan_id: string; status: string;
  /** omitted without customers.contacts.read */ contacts?: Contact[]; endpoints?: Endpoint[];
  /** omitted without customers.notes.read */ notes?: string;
  idp_linked: boolean; workspaces: number; last_intent: string | null; created_at: string;
  /** Set when the customer is moved off its plan's coverage; coverage_effective is what applies. */
  coverage: string | null; coverage_effective: string | null;
};

export type Bounds = { min?: number; max?: number; allowed?: Json[] };
export type Step = { layer: Layer; ref: string; value: Json; outcome: string; reason?: string };
export type Effective = { key: string; value: Json; source: Layer; ref: string; lock: Lock; bounds?: Bounds; note?: string; flags: string[]; trail: Step[] };
export type EffectiveConfig = { tenant_id: string; plan_id: string; layers: Layer[]; settings: Effective[]; services: Effective[]; enabled_services: string[] };

export type Intent = { id: string; tenant_id: string; status: string; ticket_ref: string; created_by: string; created_at: string; enabled_services: string[] };
export type Items<T> = { items: T[] };
