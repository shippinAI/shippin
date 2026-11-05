export type Lock = "locked" | "overridable" | "mergeable" | "bounded";
export type Layer = "system" | "plan" | "tenant" | "workspace";
export type Json = string | number | boolean | null | Json[] | { [k: string]: Json };

export type Plan = { id: string; label: string; description: string; rank: number; /** omitted by the API without catalog.prices.read */ price_cents?: number; public: boolean; active: boolean };

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

export type Contact = { name: string; email: string; role: string };
export type Endpoint = { kind: string; host: string; note: string };

export type Customer = {
  id: string; name: string; plan_id: string; status: string;
  /** omitted without customers.contacts.read */ contacts?: Contact[]; endpoints?: Endpoint[];
  /** omitted without customers.notes.read */ notes?: string;
  idp_linked: boolean; workspaces: number; last_intent: string | null; created_at: string;
};

export type Bounds = { min?: number; max?: number; allowed?: Json[] };
export type Step = { layer: Layer; ref: string; value: Json; outcome: string; reason?: string };
export type Effective = { key: string; value: Json; source: Layer; ref: string; lock: Lock; bounds?: Bounds; note?: string; flags: string[]; trail: Step[] };
export type EffectiveConfig = { tenant_id: string; plan_id: string; layers: Layer[]; settings: Effective[]; services: Effective[]; enabled_services: string[] };

export type Intent = { id: string; tenant_id: string; status: string; ticket_ref: string; created_by: string; created_at: string; enabled_services: string[] };
export type Items<T> = { items: T[] };
