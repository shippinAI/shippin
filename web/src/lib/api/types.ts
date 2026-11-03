import type { Job } from "./job";
// Mirrors api/openapi (Me, Tenant) from the shippin-cloud plan repo, v0.1.
// ⚠ Hand-typed until the generated client lands (CONSOLE-IA ground rules).
export type TenantRef = {
  tenant_id: string; name: string; roles: string[]; status: string; can_manage: boolean;
  /** Set when an admin sees this tenant only through a platform access grant. */
  access?: { kind: "request" | "breakglass"; scope: string; expires_at: string };
};

export type Me = {
  staff?: { mfa_required?: boolean };
  user_id: string;
  display_name: string;
  email?: string;
  platform_roles: string[];
  tenants: TenantRef[];
  capabilities: Record<string, boolean>;
};

export type PublicSettings = {
  brand: { name: string; short: string; tagline: string };
  domain: string;
  governance_domains: string[];
  idp_host: string;
  idp_name: string;
  /** IdP self-service page (MFA, profile); "" when not configured. */
  idp_account_url?: string;
  login_ready: boolean;
  /** Local magic-link sign-in is offered when an upstream issuer is not required. */
  local_login?: boolean;
  signup?: "open" | "invite_only" | "waitlist";
  /** True until the installation owner redeems the setup code at /setup. */
  setup_open?: boolean;
  /** Which app this origin serves: the customer console or the operator app (ops.<domain>). */
  app?: "console" | "operator";
  console_url?: string;
  /** The operator app's own origin; "" while the panel only lives under /admin. */
  operator_url?: string;
  operator_origin_only?: boolean;
  /** Upstream sign-ins (Google, GitHub...) the IdP offers; each is /auth/login?idp=<id>. */
  login_providers?: { id: string; label: string }[];
  sign_in_providers?: { id: string; label: string }[];
};

export type Probe = { name: string; kind: string; instance: string; healthy: boolean; configured?: boolean; latency_ms: number; detail: string };

export type Instance = {
  name: string;
  status: string;
  type: string;
  ipv4: string;
  memory_bytes: number;
  processes: number;
  created_at: string;
  image: string;
  autostart: boolean;
};

export type Services = {
  project: string;
  host: string;
  probes: Probe[];
  instances: Instance[];
  instances_error?: string;
  checked_at: string;
};

export type TenantSettings = {
  tenant_id: string;
  display_name: string;
  plan_name: string;
  default_domain: string;
  governance_domains: string[];
  idp_issuer: string;
  region: string;
  status: string;
  idp_org_id: string;
  incus_project: string;
  ssh_ip: string;
};

export type Problem = { status: number; message: string };

export type WorkspaceState = "unknown" | "provisioning" | "running" | "stopped" | "starting" | "stopping" | "deleting" | "failed";

export type FirewallRule = { protocol: "tcp" | "udp"; port: number; source: string };

export type SSHKey = { id: string; name: string; public_key: string; fingerprint: string; created_at: string };

export type Connect = { host: string; port: number; user: string; ssh: string; ssh_config: string; vscode: string };

export type Workspace = {
  id: string;
  tenant_id: string;
  name: string;
  state: WorkspaceState;
  observation?: "observed" | "missing" | "unavailable";
  observed_at?: string;
  recorded_state?: string;
  ssh_port: number;
  ipv4?: string;
  created_by: string;
  error?: string;
  created_at: string;
  instance: string;
  memory_bytes: number;
  ssh_key_count: number;
  connect?: Connect;
  host: string;
  url: string;
  public_ip: string;
  region: string;
  vscode_url?: string;
  ide_port: number;
  /** The provisioning job; present on the 202 from create while durable jobs are enabled. */
  job?: Job;
  /** M1 hierarchy; absent for legacy workspaces. */
  project_id?: string;
  environment_id?: string;
  domains: string[];
  firewall: FirewallRule[];
  template_id?: string;
  repo: string;
  size: WorkspaceSize;
  /** Current provisioning step (only while provisioning). */
  progress?: string;
  preview_urls: PreviewURL[];
};

export type WorkspaceSize = { cpu: number; memory_gib: number; disk_gib: number };
export type SizePreset = WorkspaceSize & { id: string; label: string };
export type Preview = { port: number; public: boolean };
export type PreviewURL = Preview & { url: string };

export type Template = {
  id: string; slug: string; name: string; description: string; icon: string; image: string;
  toolchain: string[]; cpu: number; memory_gib: number; disk_gib: number; repo: string; startup: string;
  ports: number[]; enabled: boolean; sort: number; created_at: string;
};
export type TemplateRules = { repo_hosts: string[]; image_prefix: string; preview_min: number; preview_max: number };
export type TemplateList = { items: Template[]; sizes: SizePreset[]; limits: TemplateRules };

export type PlanLimits = { workspace_cpu: number; workspace_memory: string; home_gib: number; root_gib: number };

export type WorkspaceList = {
  items: Workspace[];
  can_manage: boolean;
  tenant_status: string;
  plan: { name: string; limits?: PlanLimits };
};
