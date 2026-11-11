export type Lock = "locked" | "overridable" | "mergeable" | "bounded";
export type Layer = "system" | "plan" | "tenant" | "workspace";
export type Json = string | number | boolean | null | Json[] | { [k: string]: Json };

export type Plan = { id: string; label: string; description: string; rank: number; /** omitted by the API without catalog.prices.read */ price_cents?: number; public: boolean; active: boolean;
  /** The coverage tier this plan includes. */
  coverage: string };

type Term = "none" | "quoted" | "included";
/** Support and operation sold beside a plan. Response targets are first-response minutes within support hours; null is no target. */
export type CoverageTier = {
  id: string; label: string; description: string; rank: number;
  /** How members of organizations with this coverage must sign in. */
  sign_in: "password" | "second_factor" | "identity_provider";
  support_hours: "business" | "critical_always" | "always";
  channels: ("docs" | "community" | "email" | "chat" | "phone" | "video")[];
  critical_minutes: number | null; high_minutes: number | null; normal_minutes: number | null;
  /** Percentage such as 99.9; null is no commitment. */
  uptime_target: number | null;
  operations: "self" | "assisted" | "managed"; named_contact: boolean; reviews: "none" | "quarterly" | "monthly"; training_hours: number;
  consulting: Term; custom_deployments: Term; remote_operations: Term; team_training: Term;
  /** omitted without catalog.prices.read: the charge is the greater of the monthly minimum and the share of plan charges. */
  price_cents?: number; spend_percent?: number;
  public: boolean; active: boolean;
};

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
  /** Set when the customer is moved off its plan's coverage; coverage_effective is what applies. */
  coverage: string | null; coverage_effective: string | null;
};

export type Bounds = { min?: number; max?: number; allowed?: Json[] };
export type Step = { layer: Layer; ref: string; value: Json; outcome: string; reason?: string };
export type Effective = { key: string; value: Json; source: Layer; ref: string; lock: Lock; bounds?: Bounds; note?: string; flags: string[]; trail: Step[] };
export type EffectiveConfig = { tenant_id: string; plan_id: string; layers: Layer[]; settings: Effective[]; services: Effective[]; enabled_services: string[] };

export type Intent = { id: string; tenant_id: string; status: string; ticket_ref: string; created_by: string; created_at: string; enabled_services: string[] };
export type Items<T> = { items: T[] };
