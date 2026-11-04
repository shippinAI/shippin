import { Bot, BarChart3, Globe, SquareTerminal, FolderGit2, KeySquare, Boxes, Fingerprint, Gauge, KeyRound, LayoutGrid, Network, Server, Settings, ShieldCheck, Users, Cable, Database, Cpu, Package } from "lucide-react";
import type { LucideIcon } from "lucide-react";

export type NavItem = {
  /** Stable name; `ui.nav.items` renames, hides or moves an entry by it. */
  id: string;
  label: string;
  icon: LucideIcon;
  /** Path under /t/:tenantId, or absolute for operator routes. */
  to?: string;
  hue: string;
  soon?: string;
  /** Only shown when this installation has switched this feature on. */
  feature?: string;
  /** Only shown when /v1/me grants this capability. */
  cap?: string;
  /** Hidden when /v1/me grants this capability: the caller has a better place for it. */
  unlessCap?: string;
  /** Also active on routes under this one. */
  prefix?: boolean;
  /** Listed in the phone menu. */
  mobile?: boolean;
};

export type NavSection = { id: string; label: string };

// Order and names follow CONSOLE-IA.md "Navigation tree per role".
export const tenantNav: NavItem[] = [
  { id: "tenant.home", label: "Home", icon: LayoutGrid, to: "", hue: "var(--color-primary)", mobile: true },
  { id: "tenant.projects", label: "Projects", icon: FolderGit2, to: "projects", hue: "var(--color-hue-workspaces)", feature: "workspaces" },
  { id: "tenant.database", label: "Database", icon: Database, to: "database", hue: "var(--color-hue-data)", mobile: true, feature: "workspaces" },
  { id: "tenant.deployments", label: "Services", icon: Package, to: "deployments", hue: "var(--color-hue-data)", mobile: true, feature: "servers" },
  { id: "tenant.workspaces", label: "Workspaces", icon: Boxes, to: "workspaces", hue: "var(--color-hue-workspaces)", mobile: true, feature: "workspaces" },
  { id: "tenant.terminal", label: "Terminal", icon: SquareTerminal, to: "go/terminal", hue: "var(--color-hue-workspaces)", feature: "workspaces" },
  { id: "tenant.connect", label: "Connect accounts", icon: KeyRound, to: "go/connect", hue: "var(--color-hue-secrets)", feature: "workspaces" },
  { id: "tenant.publish", label: "Publish", icon: Globe, to: "go/publish", hue: "var(--color-hue-edge)", feature: "workspaces" },
  { id: "tenant.usage", label: "Usage", icon: BarChart3, to: "go/usage", hue: "var(--color-hue-data)", feature: "workspaces" },
  { id: "tenant.secrets", label: "Secrets", icon: KeyRound, to: "/vaultix", hue: "var(--color-hue-secrets)", feature: "secrets" },
  { id: "tenant.servers", label: "Servers", icon: Server, to: "servers", hue: "var(--color-hue-edge)", feature: "servers" },
  { id: "tenant.identity", label: "Identity", icon: Fingerprint, to: "identity", hue: "var(--color-hue-identity)" },
  { id: "tenant.network", label: "Network", icon: Network, hue: "var(--color-hue-network)", soon: "Per-org network policy lands with hypervisor pools" },
  { id: "tenant.ai", label: "AI & Agents", icon: Bot, to: "go/ai", hue: "var(--color-hue-ai)", feature: "ai" },
  { id: "tenant.team", label: "Team", icon: Users, to: "team", hue: "var(--color-hue-billing)" },
  { id: "tenant.settings", label: "Settings", icon: Settings, to: "settings", hue: "var(--color-fg-muted)" },
];

export const infrastructureNav: NavItem[] = [
  { id: "infra.hosts", label: "Hosts & nodes", icon: Cpu, to: "/operator/hosts", hue: "var(--color-hue-edge)", cap: "infra.hosts", prefix: true, mobile: true },
];

export const operatorNav: NavItem[] = [
  { id: "operator.health", label: "Platform health", icon: Gauge, to: "/operator", hue: "var(--color-hue-data)" },
  { id: "operator.instances", label: "Instances", icon: Server, to: "/operator/instances", hue: "var(--color-hue-edge)", unlessCap: "infra.hosts" },
  { id: "operator.panel", label: "Provider panel", icon: ShieldCheck, to: "/admin", hue: "var(--color-warning)", cap: "provider.panel" },
];

export const accountNav: NavItem[] = [
  { id: "account.hub", label: "Approvals", icon: LayoutGrid, to: "/hub", hue: "var(--color-hue-ai)", feature: "board" },
  { id: "account.security", label: "Security", icon: KeyRound, to: "/account/security", hue: "var(--color-hue-secrets)", cap: "account.local" },
  { id: "account.ssh", label: "SSH keys", icon: KeySquare, to: "/account/ssh-keys", hue: "var(--color-hue-secrets)", feature: "workspaces" },
  { id: "account.chat", label: "Automaton chat", icon: Bot, to: "/chat", hue: "var(--color-hue-ai)", mobile: true, feature: "chat" },
  { id: "account.ai", label: "AI subscriptions", icon: Bot, to: "/account/ai", hue: "var(--color-hue-ai)", feature: "ai" },
  { id: "account.bridge", label: "Bridge sessions", icon: Cable, to: "/account/bridge", hue: "var(--color-hue-secrets)", feature: "bridge" },
];

export const navSections = {
  devspace: { id: "devspace", label: "Devspace" },
  account: { id: "account", label: "Account" },
  infrastructure: { id: "infrastructure", label: "Infrastructure" },
  operator: { id: "operator", label: "Operator" },
} satisfies Record<string, NavSection>;

/** Whether this installation has the entry's feature switched on. */
export function featureOn(caps: Record<string, boolean>) {
  return (i: NavItem) => !i.feature || caps[`feature.${i.feature}`] === true;
}

/** Filters entries down to what /v1/me lets this caller open. */
export function allowedBy(caps: Record<string, boolean>) {
  return (i: NavItem) => (!i.cap || caps[i.cap]) && !(i.unlessCap && caps[i.unlessCap]) && featureOn(caps)(i);
}
